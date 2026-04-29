package structconfig

import (
	"errors"
	"fmt"
	"maps"
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
// ErrVersionCalled will be returned by Process when the --version flag is set.
// ErrDefaultConfigCalled will be returned by Process when the --default-config flag is set.
// ErrDebugCalled will be returned by Process when the --debug flag is set.
var (
	ErrInvalidSpecification = errors.New("specification must be a struct pointer")
	ErrVersionCalled        = errors.New("version flag was set")
	ErrDefaultConfigCalled  = errors.New("default-config flag was set")
	ErrDebugCalled          = errors.New("debug flag was set")
)

var (
	gatherRegexp  = regexp.MustCompile("([A-Z]+[a-z]*|[a-z]+|[0-9]+)")
	acronymRegexp = regexp.MustCompile("([A-Z]+)([A-Z][^A-Z]+)")
)

const (
	skipTagValue         = "-"
	skipBuiltInFlagValue = "-"
	defaultConfigType    = "toml"

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

	shortConfigPath    = "c"
	shortConfigType    = "t"
	shortDefaultConfig = "p"
	shortVersion       = "V"
	shortDebug         = "d"

	sourceDefault = "default"
	sourceFile    = "file"
	sourceEnv     = "env"
	sourceFlag    = "flag"
	sourceUnset   = "unset"
)

// keySource records the effective value and its origin for a single config key.
type keySource struct {
	Key    string
	Value  string
	Source string
}

// varInfo maintains information about the configuration variable.
type varInfo struct {
	Default     string
	typ         reflect.Type
	Name        string
	Key         string
	Env         string
	Flag        string
	ShortFlag   string
	File        string
	Description string
	Required    bool
}

// VersionFunc returns the version string used by the built-in version flag.
type VersionFunc func() string

var defaultVersionFunc VersionFunc = func() string {
	return fmt.Sprintf("Go version: %s", runtime.Version())
}

// StructConfig manages startup-time configuration loading for one Process call.
type StructConfig struct {
	flags    *pflag.FlagSet
	options  *Options
	fileData map[string]any
	infos    []varInfo
}

// Options configures StructConfig behavior.
type Options struct {
	VersionFunc VersionFunc
	ConfigType  string
	Tags        OptionTags
	FlagNames   OptionFlagNames
	FlagShorts  OptionFlagShorts
}

// OptionTags defines struct tag names used for config keys, env vars, and flags.
type OptionTags struct {
	FileTag  string
	FlagTag  string
	ShortTag string
	EnvTag   string
	DescTag  string
}

// OptionFlagNames customizes built-in long flag names.
type OptionFlagNames struct {
	ConfigPath    string
	ConfigType    string
	DefaultConfig string
	Version       string
	Debug         string
}

// OptionFlagShorts customizes built-in short flag aliases.
type OptionFlagShorts struct {
	ConfigPath    string
	ConfigType    string
	DefaultConfig string
	Version       string
	Debug         string
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

	if o.Tags.FlagTag == "" {
		o.Tags.FlagTag = tagFlag
	}

	if o.Tags.ShortTag == "" {
		o.Tags.ShortTag = tagShortFlag
	}

	if o.Tags.EnvTag == "" {
		o.Tags.EnvTag = tagEnv
	}

	if o.Tags.DescTag == "" {
		o.Tags.DescTag = tagDescription
	}

	if o.FlagNames.ConfigPath == "" {
		o.FlagNames.ConfigPath = flagConfigPath
	}

	if o.FlagNames.ConfigType == "" {
		o.FlagNames.ConfigType = flagConfigType
	}

	if o.FlagNames.DefaultConfig == "" {
		o.FlagNames.DefaultConfig = flagDefaultConfig
	}

	if o.FlagNames.Version == "" {
		o.FlagNames.Version = flagVersion
	}

	if o.FlagNames.Debug == "" {
		o.FlagNames.Debug = flagDebug
	}

	if o.FlagShorts.ConfigPath == "" {
		o.FlagShorts.ConfigPath = shortConfigPath
	}

	if o.FlagShorts.ConfigType == "" {
		o.FlagShorts.ConfigType = shortConfigType
	}

	if o.FlagShorts.DefaultConfig == "" {
		o.FlagShorts.DefaultConfig = shortDefaultConfig
	}

	if o.FlagShorts.Version == "" {
		o.FlagShorts.Version = shortVersion
	}

	if o.FlagShorts.Debug == "" {
		o.FlagShorts.Debug = shortDebug
	}

	return o
}

// gatherInfo gathers information about the specified struct.
func (s *StructConfig) gatherInfo(prefix, envPrefix string, spec any) ([]varInfo, error) {
	specValue := reflect.ValueOf(spec)

	if specValue.Kind() != reflect.Pointer {
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

		for f.Kind() == reflect.Pointer {
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
			Env:         ftype.Tag.Get(s.options.Tags.EnvTag),
			Flag:        ftype.Tag.Get(s.options.Tags.FlagTag),
			File:        ftype.Tag.Get(s.options.Tags.FileTag),
			ShortFlag:   ftype.Tag.Get(s.options.Tags.ShortTag),
			Default:     ftype.Tag.Get(tagDefault),
			Description: ftype.Tag.Get(s.options.Tags.DescTag),
			Required:    required,
			typ:         ftype.Type,
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

// NewStructConfig creates a StructConfig with the provided options.
//
// StructConfig is intended to be used once during application startup.
func NewStructConfig(o *Options) *StructConfig {
	return &StructConfig{
		flags:   pflag.NewFlagSet("flag set", pflag.ContinueOnError),
		options: o.fillDefaults(),
	}
}

// Process populates the specified struct based on environment, flags, config file,
// and default values with default options.
func Process(prefix string, spec any) (string, error) {
	return NewStructConfig(nil).Process(prefix, spec)
}

// Process populates the specified struct based on environment, flags, config file,
// and default values. Priority: flags > env vars > config file > struct tag defaults.
func (s *StructConfig) Process(prefix string, spec any) (string, error) {
	var err error

	s.infos, err = s.gatherInfo("", prefix, spec)
	if err != nil {
		if errors.Is(err, ErrInvalidSpecification) {
			return "", ErrInvalidSpecification
		}

		return "", fmt.Errorf("gather info: %w", err)
	}

	for i := range s.infos {
		err = s.addFlag(&s.infos[i])
		if err != nil {
			return "", fmt.Errorf("add flag: %w", err)
		}
	}

	err = s.addBuiltInFlags()
	if err != nil {
		return "", fmt.Errorf("add built-in flags: %w", err)
	}

	err = s.flags.Parse(os.Args[1:])
	if err != nil {
		return "", fmt.Errorf("parse flags: %w", err)
	}

	versionOut, err := s.processVersionFlag()
	if err != nil {
		return versionOut, err
	}

	configOut, err := s.processDefaultConfigFlag()
	if err != nil {
		return configOut, err
	}

	configPath, configType, err := s.getConfigPathAndType()
	if err != nil {
		return "", err
	}

	if configType != "" {
		s.options.ConfigType = configType
	}

	err = s.readConfigFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config file: %w", err)
	}

	merged, err := s.buildMerged()
	if err != nil {
		return "", err
	}

	debugOut, err := s.processDebugFlag(merged)
	if err != nil {
		return debugOut, err
	}

	if err = s.checkRequired(merged); err != nil {
		return "", err
	}

	if err = s.unmarshalInto(merged, spec); err != nil {
		return "", err
	}

	initNilMaps(reflect.ValueOf(spec).Elem())

	return "", nil
}

// buildMerged assembles a flat dot-keyed map from all sources in priority order:
// struct tag defaults < config file < environment variables < CLI flags.
func (s *StructConfig) buildMerged() (map[string]any, error) {
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

		val, err := readFlagValue(s.flags, info)
		if err != nil {
			return nil, fmt.Errorf("source flag --%s (field %q, key %q): %w", info.Flag, info.Name, info.Key, err)
		}

		m[info.Key] = val
	}

	return m, nil
}

// readFlagValue reads a typed value from a pflag flag based on the field's reflect type.
func readFlagValue(flags *pflag.FlagSet, info varInfo) (any, error) {
	typ := info.typ
	if typ.Kind() == reflect.Pointer {
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
			stringToMapStringHookFunc("=", ","),
		),
	})
	if err != nil {
		return err
	}

	return decoder.Decode(expandKeys(m))
}

func initNilMaps(v reflect.Value) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}

		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	for i := range v.NumField() {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}

		switch f.Kind() {
		case reflect.Map:
			if f.IsNil() {
				f.Set(reflect.MakeMap(f.Type()))
			}
		case reflect.Struct:
			initNilMaps(f)
		case reflect.Pointer:
			initNilMaps(f)
		}
	}
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

// MustProcess is the same as Process but exits 0 for built-in control-flow
// flags (version/default-config/debug) and panics for all other errors.
func MustProcess(prefix string, spec any) {
	if out, err := Process(prefix, spec); err != nil {
		if out != "" {
			fmt.Print(out)
		}

		if errors.Is(err, ErrVersionCalled) || errors.Is(err, ErrDefaultConfigCalled) || errors.Is(err, ErrDebugCalled) {
			os.Exit(0)
		}

		panic(err)
	}
}

// MustProcess is the same as Process but exits 0 for built-in control-flow
// flags (version/default-config/debug) and panics for all other errors.
func (s *StructConfig) MustProcess(prefix string, spec any) {
	if out, err := s.Process(prefix, spec); err != nil {
		if out != "" {
			fmt.Print(out)
		}

		if errors.Is(err, ErrVersionCalled) || errors.Is(err, ErrDefaultConfigCalled) || errors.Is(err, ErrDebugCalled) {
			os.Exit(0)
		}

		panic(err)
	}
}

func (s *StructConfig) addBuiltInFlags() error {
	err := s.addBuiltInStringFlag(s.options.FlagNames.ConfigPath, s.options.FlagShorts.ConfigPath, "", "explicit path to application config")
	if err != nil {
		return err
	}

	err = s.addBuiltInStringFlag(s.options.FlagNames.ConfigType, s.options.FlagShorts.ConfigType, s.options.ConfigType, "config file type")
	if err != nil {
		return err
	}

	err = s.addBuiltInBoolFlag(s.options.FlagNames.DefaultConfig, s.options.FlagShorts.DefaultConfig, "print default config to stdout and exit")
	if err != nil {
		return err
	}

	err = s.addBuiltInBoolFlag(s.options.FlagNames.Debug, s.options.FlagShorts.Debug, "print config debug info and exit")
	if err != nil {
		return err
	}

	return s.addBuiltInBoolFlag(s.options.FlagNames.Version, s.options.FlagShorts.Version, "print application version info and exit")
}

func (s *StructConfig) addBuiltInBoolFlag(name, short, desc string) error {
	if name == "" || name == skipBuiltInFlagValue {
		return nil
	}

	if s.flags.Lookup(name) != nil {
		return fmt.Errorf("built-in flag %q conflicts with a field flag", name)
	}

	if short != "" && s.flags.ShorthandLookup(short) != nil {
		return fmt.Errorf("built-in flag %q short %q conflicts with a field flag", name, short)
	}

	s.flags.BoolP(name, short, false, desc)

	return nil
}

func (s *StructConfig) addBuiltInStringFlag(name, short, defVal, desc string) error {
	if name == "" || name == skipBuiltInFlagValue {
		return nil
	}

	if s.flags.Lookup(name) != nil {
		return fmt.Errorf("built-in flag %q conflicts with a field flag", name)
	}

	if short != "" && s.flags.ShorthandLookup(short) != nil {
		return fmt.Errorf("built-in flag %q short %q conflicts with a field flag", name, short)
	}

	s.flags.StringP(name, short, defVal, desc)

	return nil
}

func (s *StructConfig) processVersionFlag() (string, error) {
	if s.options.FlagNames.Version == skipBuiltInFlagValue {
		return "", nil
	}

	showVersion, err := s.flags.GetBool(s.options.FlagNames.Version)
	if err != nil {
		return "", err
	}

	if showVersion {
		v := s.options.VersionFunc()
		if !strings.HasSuffix(v, "\n") {
			v += "\n"
		}

		return v, ErrVersionCalled
	}

	return "", nil
}

func (s *StructConfig) processDefaultConfigFlag() (string, error) {
	if s.options.FlagNames.DefaultConfig == skipBuiltInFlagValue {
		return "", nil
	}

	printConfig, err := s.flags.GetBool(s.options.FlagNames.DefaultConfig)
	if err != nil {
		return "", err
	}

	if !printConfig {
		return "", nil
	}

	defaults := make(map[string]any, len(s.infos))

	for _, info := range s.infos {
		if info.Default != "" {
			defaults[info.Key] = info.Default
		} else {
			defaults[info.Key] = reflect.Zero(info.typ).Interface()
		}
	}

	out, err := s.dumpConfig(expandKeys(defaults))
	if err != nil {
		return "", err
	}

	return out, ErrDefaultConfigCalled
}

// buildSourceAttribution walks each known field and records the highest-priority
// source that provided its value (default < file < env < flag).
func (s *StructConfig) buildSourceAttribution() []keySource {
	fileFlat := flattenMap("", s.fileData)
	result := make([]keySource, 0, len(s.infos))

	for _, info := range s.infos {
		ks := keySource{Key: info.Key, Value: "<unset>", Source: sourceUnset}

		if info.Default != "" {
			ks.Value = info.Default
			ks.Source = sourceDefault
		}

		if _, ok := fileFlat[info.Key]; ok {
			ks.Value = fmt.Sprint(fileFlat[info.Key])
			ks.Source = sourceFile
		}

		if info.Env != skipTagValue && info.Env != "" {
			if val, ok := os.LookupEnv(info.Env); ok {
				ks.Value = val
				ks.Source = fmt.Sprintf("%s (%s)", sourceEnv, info.Env)
			}
		}

		if info.Flag != skipTagValue && info.Flag != "" {
			f := s.flags.Lookup(info.Flag)
			if f != nil && f.Changed {
				ks.Value = f.Value.String()
				ks.Source = fmt.Sprintf("%s (--%s)", sourceFlag, info.Flag)
			}
		}

		result = append(result, ks)
	}

	return result
}

// formatSourceTable renders a fixed-width table of key/value/source rows.
func formatSourceTable(sources []keySource) string {
	const (
		hKey    = "KEY"
		hValue  = "VALUE"
		hSource = "SOURCE"
	)

	wKey, wValue, wSource := len(hKey), len(hValue), len(hSource)

	for _, ks := range sources {
		if l := len(ks.Key); l > wKey {
			wKey = l
		}

		if l := len(ks.Value); l > wValue {
			wValue = l
		}

		if l := len(ks.Source); l > wSource {
			wSource = l
		}
	}

	var b strings.Builder

	rowFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds\n", wKey, wValue, wSource)
	sepFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds\n", wKey, wValue, wSource)

	fmt.Fprintf(&b, rowFmt, hKey, hValue, hSource)
	fmt.Fprintf(&b, sepFmt,
		strings.Repeat("-", wKey),
		strings.Repeat("-", wValue),
		strings.Repeat("-", wSource),
	)

	for _, ks := range sources {
		fmt.Fprintf(&b, rowFmt, ks.Key, ks.Value, ks.Source)
	}

	return b.String()
}

func (s *StructConfig) processDebugFlag(merged map[string]any) (string, error) {
	if s.options.FlagNames.Debug == skipBuiltInFlagValue {
		return "", nil
	}

	printDebug, err := s.flags.GetBool(s.options.FlagNames.Debug)
	if err != nil {
		return "", err
	}

	if !printDebug {
		return "", nil
	}

	configOut, err := s.dumpConfig(expandKeys(merged))
	if err != nil {
		return "", err
	}

	table := formatSourceTable(s.buildSourceAttribution())

	return configOut + "\n" + table, ErrDebugCalled
}

func (s *StructConfig) dumpConfig(config map[string]any) (string, error) {
	var buf strings.Builder

	switch s.options.ConfigType {
	case "toml":
		if err := toml.NewEncoder(&buf).Encode(config); err != nil {
			return "", err
		}
	case "yaml":
		if err := yaml.NewEncoder(&buf).Encode(config); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported config type %s", s.options.ConfigType)
	}

	return buf.String(), nil
}

func (s *StructConfig) getConfigPathAndType() (string, string, error) {
	if s.options.FlagNames.ConfigPath == skipBuiltInFlagValue {
		return "", "", nil
	}

	path, err := s.flags.GetString(s.options.FlagNames.ConfigPath)
	if err != nil {
		return "", "", err
	}

	if s.options.FlagNames.ConfigType == skipBuiltInFlagValue {
		return path, "", nil
	}

	configType, err := s.flags.GetString(s.options.FlagNames.ConfigType)
	if err != nil {
		return "", "", err
	}

	return path, configType, nil
}

func (s *StructConfig) readConfigFile(path string) error {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw map[string]any

	switch s.options.ConfigType {
	case "toml":
		if err = toml.Unmarshal(data, &raw); err != nil {
			return err
		}
	case "yaml":
		if err = yaml.Unmarshal(data, &raw); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported config type %q", s.options.ConfigType)
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
			maps.Copy(out, flattenMap(key, nested))
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

	if s.flags.Lookup(v.Flag) != nil {
		return fmt.Errorf("found redefined flag for %q", v.Flag)
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
			s.flags.DurationP(v.Flag, v.ShortFlag, 0, descr)
		} else {
			s.flags.Int64P(v.Flag, v.ShortFlag, 0, descr)
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

// stringToMapStringHookFunc converts a delimited key/value string into a
// map[string]T value for mapstructure decode targets.
func stringToMapStringHookFunc(kvSep, sep string) mapstructure.DecodeHookFunc {
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
