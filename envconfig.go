package structconfig

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
	Default     any
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

type VersionFunc func() string

var defaultVersionFunc VersionFunc = func() string {
	return fmt.Sprintf("Go version: %s", runtime.Version())
}

type StructConfig struct {
	viper     *viper.Viper
	flags     *pflag.FlagSet
	options   *Options
	infos     []varInfo
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

	// over allocate an info array, we will extend if needed later
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
					// nil pointer to a non-struct: leave it alone
					break
				}
				// nil pointer to struct: create a zero instance
				f.Set(reflect.New(f.Type().Elem()))
			}
			f = f.Elem()
		}

		required, err := isTrue2(ftype.Tag.Get(tagRequired))
		if err != nil {
			return nil, fmt.Errorf("bad required tag value for field %s: %w", ftype.Name, err)
		}

		// Capture information about the config variable
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
		}

		// If file tag present, it will be used as default for key and env variable
		if info.File != "" {
			info.Name = info.File
		}
		info.Key = info.Name

		if prefix != "" {
			info.Key = prefix + "." + info.Key
		}
		info.Key = strings.ToLower(info.Key)

		// Default to the field name as the env var name (will be upcased)
		if info.Env == "" {
			name := splitWords(info.Name, isTrue(ftype.Tag.Get(tagSplitWords)))

			if envPrefix != "" {
				info.Env = strings.ToUpper(envPrefix + "_" + name)
			} else {
				info.Env = strings.ToUpper(name)
			}
		}

		// Default to the field name as the flag var name
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

	// Best effort to un-pick camel casing as separate words
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
	v := viper.New()
	flags := pflag.NewFlagSet("flag set", pflag.ContinueOnError)

	return &StructConfig{
		viper:   v,
		flags:   flags,
		options: o.fillDefaults(),
	}
}

// Process populates the specified struct based on
// environment, flags, config file, default values
// with default options.
func Process(prefix string, spec any) error {
	defaultConfig := NewStructConfig(nil)
	return defaultConfig.Process(prefix, spec)
}

// Process populates the specified struct based on
// environment, flags, config file, default values.
func (s *StructConfig) Process(prefix string, spec any) error {
	var err error

	s.infos, err = s.gatherInfo("", prefix, spec)
	if err != nil {
		return fmt.Errorf("gather info: %w", err)
	}

	for _, info := range s.infos {
		s.addDefaultValue(&info)

		err = s.addFlag(&info)
		if err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		err = s.addEnv(&info)
		if err != nil {
			return fmt.Errorf("add env: %w", err)
		}
	}

	s.addBuiltInFlags()

	err = s.flags.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	err = s.processVersionFlag()
	if err != nil {
		return fmt.Errorf("process version: %w", err)
	}

	err = s.processDefaultConfigFlag(spec)
	if err != nil {
		return fmt.Errorf("process default config: %w", err)
	}

	err = s.setConfigPathAndType()
	if err != nil {
		return fmt.Errorf("set config path/type: %w", err)
	}

	err = s.readConfig()
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	err = s.checkRequired()
	if err != nil {
		return fmt.Errorf("check required: %w", err)
	}

	return s.viper.Unmarshal(spec, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		StringToMapStringHookFunc("=", ","),
	)), func(c *mapstructure.DecoderConfig) {
		c.TagName = s.options.Tags.FileTag
	})
}

func (s *StructConfig) addDefaultValue(v *varInfo) {
	if v.Default != "" {
		s.viper.SetDefault(v.Key, v.Default)
	}
}

func (s *StructConfig) addFlag(v *varInfo) error {
	if v.Flag == skipTagValue || v.Flag == "" {
		return nil
	}

	if v.ShortFlag == skipTagValue {
		v.ShortFlag = ""
	}

	flag := v.Flag
	shortFlag := v.ShortFlag

	if s.flags.Lookup(v.Flag) != nil {
		return fmt.Errorf("found redefined flag or embedded struct with same fields %q - define explicit and different flags for them", flag)
	}

	if s.flags.ShorthandLookup(shortFlag) != nil {
		return fmt.Errorf("found redefined shorthand for %q - define flags for fields", shortFlag)
	}

	descr := fmt.Sprintf("key: %s, env: %s, default: [%s]", v.Key, v.Env, v.Default)

	if v.Description != "" {
		descr = fmt.Sprintf("%s\n%s", descr, v.Description)
	}

	if v.typ.Kind() == reflect.Ptr {
		v.typ = v.typ.Elem()
	}

	switch v.typ.Kind() {
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
		if v.typ.PkgPath() == "time" && v.typ.Name() == "Duration" {
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
		if v.typ.Key().Kind() != reflect.String {
			return fmt.Errorf("unsupported key type for maps %s for flag %s(%s)", v.typ, v.Name, v.Flag)
		}

		switch v.typ.Elem().Kind() {
		case reflect.String:
			s.flags.StringToStringP(v.Flag, v.ShortFlag, map[string]string{}, descr)
		case reflect.Int:
			s.flags.StringToIntP(v.Flag, v.ShortFlag, map[string]int{}, descr)
		case reflect.Int64:
			s.flags.StringToInt64P(v.Flag, v.ShortFlag, map[string]int64{}, descr)
		default:
			return fmt.Errorf("unsupported element type for maps %s for flag %s(%s)", v.typ, v.Name, v.Flag)
		}
	default:
		return fmt.Errorf("unsupported type %s for flag %s(%s)", v.typ, v.Name, v.Flag)
	}

	err := s.viper.BindPFlag(v.Key, s.flags.Lookup(v.Flag))
	return err
}

func (s *StructConfig) addEnv(v *varInfo) error {
	if v.Env == skipTagValue {
		return nil
	}

	var err error
	if v.Env != "" {
		err = s.viper.BindEnv(v.Key, v.Env)
	} else {
		err = s.viper.BindEnv(v.Key)
	}

	if err != nil {
		return fmt.Errorf("bind key %s(%s): %w", v.Name, v.Env, err)
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
	s.flags.StringP(flagConfigType, "t", s.options.ConfigType, "config type type")
	s.flags.BoolP(flagDefaultConfig, "p", false, "print default config to stdout and exit")
	s.flags.BoolP(s.options.FlagNames.Debug, "d", false, "print config debug info and exit")
	s.flags.BoolP(flagVersion, "V", false, "print application version info and exit")
}

// processVersionFlag print version and exit.
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

func (s *StructConfig) processDefaultConfigFlag(target any) error {
	printConfig, err := s.flags.GetBool(flagDefaultConfig)
	if err != nil {
		return err
	}

	if printConfig {
		// initialize config with defaults only
		v := viper.New()
		for _, i := range s.infos {
			if i.Default != "" {
				v.SetDefault(i.Key, i.Default)
			} else {
				v.SetDefault(i.Key, reflect.Zero(i.typ).Interface())
			}
		}

		err = v.Unmarshal(target, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			StringToMapStringHookFunc("=", ","),
		)), func(c *mapstructure.DecoderConfig) {
			c.TagName = s.options.Tags.FileTag
		})
		if err != nil {
			return err
		}

		output := v.AllSettings()

		err = s.dumpConfig(output)
		if err != nil {
			return err
		}

		os.Exit(0)
	}

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

func (s *StructConfig) setConfigPathAndType() error {
	configPath, err := s.flags.GetString(flagConfigPath)
	if err != nil {
		return err
	}

	if configPath != "" {
		configType, err := s.flags.GetString(flagConfigType)
		if err != nil {
			return err
		}

		s.viper.SetConfigFile(configPath)
		s.viper.SetConfigType(configType)

		return nil
	}

	s.viper.SetConfigName(s.options.FileName)

	return nil
}

func (s *StructConfig) readConfig() error {
	err := s.viper.ReadInConfig()
	if err == nil {
		return nil
	}

	uce := new(viper.UnsupportedConfigError)
	if errors.As(err, uce) {
		s.fileError = err
		return nil
	}

	cpe := new(viper.ConfigParseError)
	if errors.As(err, cpe) {
		s.fileError = err
		return nil
	}

	cfne := new(viper.ConfigFileNotFoundError)
	if errors.As(err, cfne) {
		s.fileError = errors.Join(cfne, os.ErrNotExist)
		return nil
	}

	return err
}

func (s *StructConfig) checkRequired() error {
	for _, i := range s.infos {
		if i.Required {
			if s.viper.Get(i.Key) == nil {
				return fmt.Errorf("value for field %s(%s) is required", i.Name, i.Key)
			}
		}
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

func StringToMapStringHookFunc(kvSep, sep string) mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data any) (any, error) {
		if t.Kind() != reflect.Map || f.Kind() != reflect.String {
			return data, nil
		}

		if t.Key().Kind() != reflect.String {
			return data, nil
		}

		raw, _ := data.(string)

		switch t.Elem().Kind() {
		case reflect.String:
			return parseDefaultMapString(raw, kvSep, sep)
		case reflect.Int:
			return parseDefaultMapInt(raw, kvSep, sep)
		case reflect.Int64:
			return parseDefaultMapInt64(raw, kvSep, sep)
		default:
			return data, nil
		}
	}
}

func parseDefaultMapString(val, kvSep, sep string) (map[string]string, error) {
	if val == "" {
		return map[string]string{}, nil
	}

	ss := strings.Split(val, sep)
	out := make(map[string]string, len(ss))

	for _, pair := range ss {
		kv := strings.SplitN(pair, kvSep, 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("%s must be formatted as key%svalue", pair, kvSep)
		}

		out[kv[0]] = kv[1]
	}

	return out, nil
}

func parseDefaultMapInt(val, kvSep, sep string) (map[string]int, error) {
	if val == "" {
		return map[string]int{}, nil
	}

	ss := strings.Split(val, sep)
	out := make(map[string]int, len(ss))

	var err error

	for _, pair := range ss {
		kv := strings.SplitN(pair, kvSep, 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("%s must be formatted as key%svalue", pair, kvSep)
		}

		out[kv[0]], err = strconv.Atoi(kv[1])
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

func parseDefaultMapInt64(val, kvSep, sep string) (map[string]int64, error) {
	if val == "" {
		return map[string]int64{}, nil
	}

	ss := strings.Split(val, sep)
	out := make(map[string]int64, len(ss))

	var err error

	for _, pair := range ss {
		kv := strings.SplitN(pair, kvSep, 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("%s must be formatted as key%svalue", pair, kvSep)
		}

		out[kv[0]], err = strconv.ParseInt(kv[1], 10, 64)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}
