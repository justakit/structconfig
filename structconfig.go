package structconfig

import (
	"maps"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// ErrInvalidSpecification indicates that a specification is of the wrong type.
var ErrInvalidSpecification = errors.New("specification must be a struct pointer")

var gatherRegexp = regexp.MustCompile("([A-Z]+[a-z]*|[a-z]+|[0-9]+)")
var acronymRegexp = regexp.MustCompile("([A-Z]+)([A-Z][^A-Z]+)")

const (
	skipTagValue      = "-"
	defaultConfigType = "toml"

	tagRequired    = "required"
	tagEnv         = "env"
	tagFlag        = "flag"
	tagShortFlag   = "short"
	tagFile        = "file"
	tagDefault     = "default"
	tagDescription = "desc"
	tagIgnored     = "ignored"
	tagSplitWords  = "split_words"

	flagConfigPath    = "config"
	flagConfigType    = "config-type"
	flagDefaultConfig = "default-config"
	flagVersion       = "version"
	flagDebug         = "debug"
)

// varInfo maintains information about the configuration variable.
type varInfo struct {
	Default     string
	typ         reflect.Type
	field       reflect.Value
	Name        string
	Key         string
	Env         string
	Flag        string
	ShortFlag   string
	File        string
	Description string
	Required    bool
}

type VersionFunc func() string

var defaultVersionFunc VersionFunc = func() string {
	return fmt.Sprintf("Go version: %s", runtime.Version())
}

type StructConfig struct {
	flags     *pflag.FlagSet
	options   *Options
	infos     []varInfo
	fileData  map[string]any
	fileError error
}

type Options struct {
	VersionFunc VersionFunc
	ConfigType  string
	FileName    string
	Tags        OptionTags
	FlagNames   OptionFlagNames
}

type OptionTags struct {
	FileTag string
}

type OptionFlagNames struct {
	Debug string
}

func (o *Options) fillDefaults() *Options {
	if o == nil {
		o = &Options{}
	}

	if o.VersionFunc == nil {
		o.VersionFunc = defaultVersionFunc
	}

	if o.ConfigType == "" {
		o.ConfigType = defaultConfigType
	}

	if o.Tags.FileTag == "" {
		o.Tags.FileTag = tagFile
	}

	return o
}

// gatherInfo gathers information about the specified struct.
func (s *StructConfig) gatherInfo(prefix, envPrefix string, spec any) ([]varInfo, error) {
	specValue := reflect.ValueOf(spec)

	if specValue.Kind() != reflect.Ptr {
		return nil, ErrInvalidSpecification
	}

	specValue = specValue.Elem()
	if specValue.Kind() != reflect.Struct {
		return nil, ErrInvalidSpecification
	}

	typeOfSpec := specValue.Type()

	infos := make([]varInfo, 0, specValue.NumField())
	for i := range specValue.NumField() {
		f := specValue.Field(i)
		ftype := typeOfSpec.Field(i)
		if !f.CanSet() || isTrue(ftype.Tag.Get(tagIgnored)) {
			continue
		}

		for f.Kind() == reflect.Ptr {
			if f.IsNil() {
				if f.Type().Elem().Kind() != reflect.Struct {
					break
				}
				f.Set(reflect.New(f.Type().Elem()))
			}
			f = f.Elem()
		}

		required, err := isTrue2(ftype.Tag.Get(tagRequired))
		if err != nil {
			return nil, fmt.Errorf("bad required tag value for field %s: %w", ftype.Name, err)
		}

		info := varInfo{
			Name:        ftype.Name,
			Env:         ftype.Tag.Get(tagEnv),
			Flag:        ftype.Tag.Get(tagFlag),
			File:        ftype.Tag.Get(s.options.Tags.FileTag),
			ShortFlag:   ftype.Tag.Get(tagShortFlag),
			Default:     ftype.Tag.Get(tagDefault),
			Description: ftype.Tag.Get(tagDescription),
			Required:    required,
			typ:         ftype.Type,
			field:       f,
		}

		if info.File != "" {
			info.Name = info.File
		}
		info.Key = info.Name

		if prefix != "" {
			info.Key = prefix + "." + info.Key
		}
		info.Key = strings.ToLower(info.Key)

		if info.Env == "" {
			name := splitWords(info.Name, isTrue(ftype.Tag.Get(tagSplitWords)))

			if envPrefix != "" {
				info.Env = strings.ToUpper(envPrefix + "_" + name)
			} else {
				info.Env = strings.ToUpper(name)
			}
		}

		if info.Flag == "" {
			info.Flag = strings.ReplaceAll(info.Key, ".", "-")
		}

		infos = append(infos, info)

		if f.Kind() == reflect.Struct {
			innerPrefix := prefix
			innerEnvPrefix := envPrefix
			if !ftype.Anonymous {
				innerPrefix = info.Key
				innerEnvPrefix = info.Env
			}

			embeddedPtr := f.Addr().Interface()
			embeddedInfos, err := s.gatherInfo(innerPrefix, innerEnvPrefix, embeddedPtr)
			if err != nil {
				return nil, err
			}
			infos = append(infos[:len(infos)-1], embeddedInfos...)

			continue
		}
	}
	return infos, nil
}

func splitWords(key string, split bool) string {
	if !split {
		return key
	}

	words := gatherRegexp.FindAllStringSubmatch(key, -1)
	if len(words) > 0 {
		var name []string
		for _, words := range words {
			if m := acronymRegexp.FindStringSubmatch(words[0]); len(m) == 3 {
				name = append(name, m[1], m[2])
			} else {
				name = append(name, words[0])
			}
		}

		return strings.Join(name, "_")
	}

	return key
}

func NewStructConfig(o *Options) *StructConfig {
	return &StructConfig{
		flags:   pflag.NewFlagSet("flag set", pflag.ContinueOnError),
		options: o.fillDefaults(),
	}
}

// Process populates the specified struct based on environment, flags, config file,
// and default values with default options.
func Process(prefix string, spec any) error {
	return NewStructConfig(nil).Process(prefix, spec)
}

// Process populates the specified struct based on environment, flags, config file,
// and default values. Priority: flags > env vars > config file > struct tag defaults.
func (s *StructConfig) Process(prefix string, spec any) error {
	var err error

	s.infos, err = s.gatherInfo("", prefix, spec)
	if err != nil {
		if errors.Is(err, ErrInvalidSpecification) {
			return ErrInvalidSpecification
		}
		return fmt.Errorf("gather info: %w", err)
	}

	for i := range s.infos {
		if err = s.addFlag(&s.infos[i]); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}
	}

	s.addBuiltInFlags()

	if err = s.flags.Parse(os.Args[1:]); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	if err = s.processVersionFlag(); err != nil {
		return fmt.Errorf("process version: %w", err)
	}

	if err = s.processDefaultConfigFlag(); err != nil {
		return fmt.Errorf("process default config: %w", err)
	}

	configPath, configType, err := s.getConfigPathAndType()
	if err != nil {
		return err
	}

	if err = s.readConfigFile(configPath, configType); err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	merged := s.buildMerged()

	if err = s.checkRequired(merged); err != nil {
		return err
	}

	return s.unmarshalInto(merged, spec)
}

// buildMerged assembles a flat dot-keyed map from all sources in priority order:
// struct tag defaults < config file < environment variables < CLI flags.
func (s *StructConfig) buildMerged() map[string]any {
	m := make(map[string]any, len(s.infos))

	for _, info := range s.infos {
		if info.Default != "" {
			m[info.Key] = info.Default
		}
	}

	maps.Copy(m, flattenMap("", s.fileData))

	for _, info := range s.infos {
		if info.Env == skipTagValue || info.Env == "" {
			continue
		}
		if val, ok := os.LookupEnv(info.Env); ok {
			m[info.Key] = val
		}
	}

	for _, info := range s.infos {
		if info.Flag == skipTagValue || info.Flag == "" {
			continue
		}
		f := s.flags.Lookup(info.Flag)
		if f == nil || !f.Changed {
			continue
		}
		if val, err := readFlagValue(s.flags, info); err == nil {
			m[info.Key] = val
		}
	}

	return m
}

// readFlagValue reads a typed value from a pflag flag based on the field's reflect type.
func readFlagValue(flags *pflag.FlagSet, info varInfo) (any, error) {
	typ := info.typ
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		return flags.GetString(info.Flag)
	case reflect.Bool:
		return flags.GetBool(info.Flag)
	case reflect.Int:
		return flags.GetInt(info.Flag)
	case reflect.Int8:
		return flags.GetInt8(info.Flag)
	case reflect.Int16:
		return flags.GetInt16(info.Flag)
	case reflect.Int32:
		return flags.GetInt32(info.Flag)
	case reflect.Int64:
		if typ.PkgPath() == "time" && typ.Name() == "Duration" {
			return flags.GetDuration(info.Flag)
		}
		return flags.GetInt64(info.Flag)
	case reflect.Uint:
		return flags.GetUint(info.Flag)
	case reflect.Uint8:
		return flags.GetUint8(info.Flag)
	case reflect.Uint16:
		return flags.GetUint16(info.Flag)
	case reflect.Uint32:
		return flags.GetUint32(info.Flag)
	case reflect.Uint64:
		return flags.GetUint64(info.Flag)
	case reflect.Float32:
		return flags.GetFloat32(info.Flag)
	case reflect.Float64:
		return flags.GetFloat64(info.Flag)
	case reflect.Slice:
		return flags.GetStringSlice(info.Flag)
	case reflect.Map:
		switch typ.Elem().Kind() {
		case reflect.String:
			return flags.GetStringToString(info.Flag)
		case reflect.Int:
			return flags.GetStringToInt(info.Flag)
		case reflect.Int64:
			return flags.GetStringToInt64(info.Flag)
		default:
			return nil, fmt.Errorf("unsupported map element type %s", typ)
		}
	default:
		return flags.Lookup(info.Flag).Value.String(), nil
	}
}

func (s *StructConfig) unmarshalInto(m map[string]any, target any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           target,
		TagName:          s.options.Tags.FileTag,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			stringToTypedSliceHookFunc(","),
			StringToMapStringHookFunc("=", ","),
		),
	})
	if err != nil {
		return err
	}
	return decoder.Decode(expandKeys(m))
}

func (s *StructConfig) checkRequired(merged map[string]any) error {
	for _, info := range s.infos {
		if info.Required {
			if _, ok := merged[info.Key]; !ok {
				return fmt.Errorf("value for field %s(%s) is required", info.Name, info.Key)
			}
		}
	}
	return nil
}

// MustProcess is the same as Process but panics if an error occurs.
func MustProcess(prefix string, spec interface{}) {
	if err := Process(prefix, spec); err != nil {
		panic(err)
	}
}

// MustProcess is the same as Process but panics if an error occurs.
func (s *StructConfig) MustProcess(prefix string, spec interface{}) {
	if err := s.Process(prefix, spec); err != nil {
		panic(err)
	}
}

func (s *StructConfig) addBuiltInFlags() {
	s.flags.StringP(flagConfigPath, "c", "", "explicit path to application config")
	s.flags.StringP(flagConfigType, "t", s.options.ConfigType, "config file type")
	s.flags.BoolP(flagDefaultConfig, "p", false, "print default config to stdout and exit")
	s.flags.BoolP(s.options.FlagNames.Debug, "d", false, "print config debug info and exit")
	s.flags.BoolP(flagVersion, "V", false, "print application version info and exit")
}

func (s *StructConfig) processVersionFlag() error {
	showVersion, err := s.flags.GetBool(flagVersion)
	if err != nil {
		return err
	}

	if showVersion {
		fmt.Println(s.options.VersionFunc())
		os.Exit(0)
	}

	return nil
}

func (s *StructConfig) processDefaultConfigFlag() error {
	printConfig, err := s.flags.GetBool(flagDefaultConfig)
	if err != nil {
		return err
	}

	if !printConfig {
		return nil
	}

	defaults := make(map[string]any, len(s.infos))
	for _, info := range s.infos {
		if info.Default != "" {
			defaults[info.Key] = info.Default
		} else {
			defaults[info.Key] = reflect.Zero(info.typ).Interface()
		}
	}

	if err = s.dumpConfig(expandKeys(defaults)); err != nil {
		return err
	}

	os.Exit(0)
	return nil
}

func (s *StructConfig) dumpConfig(config map[string]any) error {
	switch s.options.ConfigType {
	case "toml":
		return toml.NewEncoder(os.Stdout).Encode(config)
	case "yaml":
		return yaml.NewEncoder(os.Stdout).Encode(config)
	default:
		return fmt.Errorf("unsupported config type %s", s.options.ConfigType)
	}
}

func (s *StructConfig) getConfigPathAndType() (string, string, error) {
	path, err := s.flags.GetString(flagConfigPath)
	if err != nil {
		return "", "", err
	}
	configType, err := s.flags.GetString(flagConfigType)
	if err != nil {
		return "", "", err
	}
	return path, configType, nil
}

func (s *StructConfig) readConfigFile(path, configType string) error {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.fileError = errors.Join(err, os.ErrNotExist)
		return nil
	}
	if err != nil {
		return err
	}

	var raw map[string]any
	switch configType {
	case "toml":
		if err = toml.Unmarshal(data, &raw); err != nil {
			s.fileError = err
			return nil
		}
	case "yaml":
		if err = yaml.Unmarshal(data, &raw); err != nil {
			s.fileError = err
			return nil
		}
	default:
		s.fileError = fmt.Errorf("unsupported config type %s", configType)
		return nil
	}

	s.fileData = raw
	return nil
}

// flattenMap converts a nested map into a flat dot-keyed map with lowercase keys.
func flattenMap(prefix string, m map[string]any) map[string]any {
	out := make(map[string]any)
	for k, v := range m {
		key := strings.ToLower(k)
		if prefix != "" {
			key = prefix + "." + key
		}
		if nested, ok := v.(map[string]any); ok {
			for nk, nv := range flattenMap(key, nested) {
				out[nk] = nv
			}
		} else {
			out[key] = v
		}
	}
	return out
}

// expandKeys converts a flat dot-keyed map into a nested map for mapstructure.
func expandKeys(flat map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range flat {
		parts := strings.Split(k, ".")
		cur := out
		for i, p := range parts {
			if i == len(parts)-1 {
				cur[p] = v
			} else {
				if _, exists := cur[p]; !exists {
					cur[p] = map[string]any{}
				}
				cur = cur[p].(map[string]any)
			}
		}
	}
	return out
}

func (s *StructConfig) addFlag(v *varInfo) error {
	if v.Flag == skipTagValue || v.Flag == "" {
		return nil
	}

	if v.ShortFlag == skipTagValue {
		v.ShortFlag = ""
	}

	// Skip if already registered (anonymous embedded structs can share flag names).
	if s.flags.Lookup(v.Flag) != nil {
		return nil
	}

	if v.ShortFlag != "" && s.flags.ShorthandLookup(v.ShortFlag) != nil {
		return fmt.Errorf("found redefined shorthand for %q - define flags for fields", v.ShortFlag)
	}

	descr := fmt.Sprintf("key: %s, env: %s, default: [%s]", v.Key, v.Env, v.Default)
	if v.Description != "" {
		descr += "\n" + v.Description
	}

	typ := v.typ
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		s.flags.StringP(v.Flag, v.ShortFlag, "", descr)
	case reflect.Bool:
		s.flags.BoolP(v.Flag, v.ShortFlag, false, descr)
	case reflect.Int:
		s.flags.IntP(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Int8:
		s.flags.Int8P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Int16:
		s.flags.Int16P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Int32:
		s.flags.Int32P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Int64:
		if typ.PkgPath() == "time" && typ.Name() == "Duration" {
			s.flags.Duration(v.Flag, 0, descr)
		} else {
			s.flags.Int64(v.Flag, 0, descr)
		}
	case reflect.Uint:
		s.flags.UintP(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Uint8:
		s.flags.Uint8P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Uint16:
		s.flags.Uint16P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Uint32:
		s.flags.Uint32P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Uint64:
		s.flags.Uint64P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Float32:
		s.flags.Float32P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Float64:
		s.flags.Float64P(v.Flag, v.ShortFlag, 0, descr)
	case reflect.Slice:
		s.flags.StringSliceP(v.Flag, v.ShortFlag, []string{}, descr)
	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			return fmt.Errorf("unsupported key type for maps %s for flag %s(%s)", typ, v.Name, v.Flag)
		}
		switch typ.Elem().Kind() {
		case reflect.String:
			s.flags.StringToStringP(v.Flag, v.ShortFlag, map[string]string{}, descr)
		case reflect.Int:
			s.flags.StringToIntP(v.Flag, v.ShortFlag, map[string]int{}, descr)
		case reflect.Int64:
			s.flags.StringToInt64P(v.Flag, v.ShortFlag, map[string]int64{}, descr)
		default:
			return fmt.Errorf("unsupported element type for maps %s for flag %s(%s)", typ, v.Name, v.Flag)
		}
	default:
		return fmt.Errorf("unsupported type %s for flag %s(%s)", typ, v.Name, v.Flag)
	}

	return nil
}

func isTrue(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}

func isTrue2(s string) (bool, error) {
	if s == "" {
		return false, nil
	}
	return strconv.ParseBool(s)
}

// stringToTypedSliceHookFunc converts a comma-separated string to []string for any
// slice target, letting mapstructure's WeaklyTypedInput handle element conversion.
func stringToTypedSliceHookFunc(sep string) mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String || t.Kind() != reflect.Slice {
			return data, nil
		}
		raw := data.(string)
		if raw == "" {
			return []string{}, nil
		}
		return strings.Split(raw, sep), nil
	}
}

func StringToMapStringHookFunc(kvSep, sep string) mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t.Kind() != reflect.Map || f.Kind() != reflect.String {
			return data, nil
		}
		if t.Key().Kind() != reflect.String {
			return data, nil
		}
		raw, _ := data.(string)
		switch t.Elem().Kind() {
		case reflect.String:
			return parseDefaultMap(raw, kvSep, sep, func(s string) (string, error) { return s, nil })
		case reflect.Int:
			return parseDefaultMap(raw, kvSep, sep, strconv.Atoi)
		case reflect.Int64:
			return parseDefaultMap(raw, kvSep, sep, func(s string) (int64, error) {
				return strconv.ParseInt(s, 10, 64)
			})
		default:
			return data, nil
		}
	}
}

func parseDefaultMap[V any](val, kvSep, sep string, convert func(string) (V, error)) (map[string]V, error) {
	if val == "" {
		return map[string]V{}, nil
	}
	ss := strings.Split(val, sep)
	out := make(map[string]V, len(ss))
	for _, pair := range ss {
		kv := strings.SplitN(pair, kvSep, 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("%s must be formatted as key%svalue", pair, kvSep)
		}
		v, err := convert(kv[1])
		if err != nil {
			return nil, err
		}
		out[kv[0]] = v
	}
	return out, nil
}
