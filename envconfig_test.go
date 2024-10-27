package structconfig

import (
	"os"
	"testing"
	"time"
)

type Specification struct {
	Embedded                     `envconfig:",squash" desc:"can we document a struct"`
	EmbeddedButIgnored           `ignored:"true"`
	Debug                        bool
	Port                         int
	Rate                         float32
	User                         string
	TTL                          uint32
	Timeout                      time.Duration
	AdminUsers                   []string
	MagicNumbers                 []int
	EmptyNumbers                 []int
	ColorCodes                   map[string]int
	MultiWordVar                 string
	MultiWordVarWithAutoSplit    uint32 `split_words:"true"`
	MultiWordACRWithAutoSplit    uint32 `split_words:"true"`
	SomePointer                  *string
	SomePointerWithDefault       *string `default:"foo2baz" desc:"foorbar is the word"`
	MultiWordVarWithAlt          string  `envconfig:"MULTI_WORD_VAR_WITH_ALT" desc:"what alt"`
	MultiWordVarWithLowerCaseAlt string  `envconfig:"multi_word_var_with_lower_case_alt"`
	NoPrefixWithAlt              string  `env:"SERVICE_HOST"`
	DefaultVar                   string  `default:"foobar"`
	RequiredVar                  string  `required:"True"`
	NoPrefixDefault              string  `envconfig:"BROKER" default:"127.0.0.1"`
	RequiredDefault              string  `required:"true" default:"foo2bar"`
	Ignored                      string  `ignored:"true"`
	NestedSpecification          struct {
		Property            string `envconfig:"inner"`
		PropertyWithDefault string `default:"fuzzybydefault"`
		IntProperty         int
	} `envconfig:"outer"`
	AfterNested string
	MapField    map[string]string `default:"one=two,three=four"`
}

type Embedded struct {
	Enabled             bool `desc:"some embedded value"`
	EmbeddedPort        int
	MultiWordVar        string `envconfig:"multiWordVarNested" desc:"nested"`
	MultiWordVarWithAlt string `envconfig:"multiWordVarNestedAlt"`
	EmbeddedAlt         string `envconfig:"EMBEDDED_WITH_ALT"`
	EmbeddedIgnored     string `ignored:"true"`
}

type EmbeddedButIgnored struct {
	FirstEmbeddedButIgnored  string
	SecondEmbeddedButIgnored string
}

func TestProcess(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_DEBUG", "true")
	os.Setenv("ENV_CONFIG_PORT", "8080")
	os.Setenv("ENV_CONFIG_RATE", "0.5")
	os.Setenv("ENV_CONFIG_USER", "Kelsey")
	os.Setenv("ENV_CONFIG_TIMEOUT", "2m")
	os.Setenv("ENV_CONFIG_ADMINUSERS", "John,Adam,Will")
	os.Setenv("ENV_CONFIG_MAGICNUMBERS", "5,10,20")
	os.Setenv("ENV_CONFIG_EMPTYNUMBERS", "")
	os.Setenv("ENV_CONFIG_COLORCODES", "red=1,green=2,blue=3")
	os.Setenv("SERVICE_HOST", "127.0.0.1")
	os.Setenv("ENV_CONFIG_TTL", "30")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")
	os.Setenv("ENV_CONFIG_IGNORED", "was-not-ignored")
	os.Setenv("ENV_CONFIG_OUTER_INNER", "iamnested")
	os.Setenv("ENV_CONFIG_AFTERNESTED", "after")
	os.Setenv("ENV_CONFIG_HONOR", "honor")
	os.Setenv("ENV_CONFIG_DATETIME", "2016-08-16T18:57:05Z")
	os.Setenv("ENV_CONFIG_MULTI_WORD_VAR_WITH_AUTO_SPLIT", "24")
	os.Setenv("ENV_CONFIG_MULTI_WORD_ACR_WITH_AUTO_SPLIT", "25")
	os.Setenv("ENV_CONFIG_URLVALUE", "https://github.com/justakit/structconfig")
	os.Setenv("ENV_CONFIG_URLPOINTER", "https://github.com/justakit/structconfig")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	err := config.Process("env_config", &s)
	if err != nil {
		t.Error(err.Error())
	}
	if s.NoPrefixWithAlt != "127.0.0.1" {
		t.Errorf("expected %v, got %v", "127.0.0.1", s.NoPrefixWithAlt)
	}
	if !s.Debug {
		t.Errorf("expected %v, got %v", true, s.Debug)
	}
	if s.Port != 8080 {
		t.Errorf("expected %d, got %v", 8080, s.Port)
	}
	if s.Rate != 0.5 {
		t.Errorf("expected %f, got %v", 0.5, s.Rate)
	}
	if s.TTL != 30 {
		t.Errorf("expected %d, got %v", 30, s.TTL)
	}
	if s.User != "Kelsey" {
		t.Errorf("expected %s, got %s", "Kelsey", s.User)
	}
	if s.Timeout != 2*time.Minute {
		t.Errorf("expected %s, got %s", 2*time.Minute, s.Timeout)
	}
	if s.RequiredVar != "foo" {
		t.Errorf("expected %s, got %s", "foo", s.RequiredVar)
	}
	if len(s.AdminUsers) != 3 ||
		s.AdminUsers[0] != "John" ||
		s.AdminUsers[1] != "Adam" ||
		s.AdminUsers[2] != "Will" {
		t.Errorf("expected %#v, got %#v", []string{"John", "Adam", "Will"}, s.AdminUsers)
	}
	if len(s.MagicNumbers) != 3 ||
		s.MagicNumbers[0] != 5 ||
		s.MagicNumbers[1] != 10 ||
		s.MagicNumbers[2] != 20 {
		t.Errorf("expected %#v, got %#v", []int{5, 10, 20}, s.MagicNumbers)
	}
	if len(s.EmptyNumbers) != 0 {
		t.Errorf("expected %#v, got %#v", []int{}, s.EmptyNumbers)
	}

	if s.Ignored != "" {
		t.Errorf("expected empty string, got %#v", s.Ignored)
	}

	if len(s.ColorCodes) != 3 ||
		s.ColorCodes["red"] != 1 ||
		s.ColorCodes["green"] != 2 ||
		s.ColorCodes["blue"] != 3 {
		t.Errorf(
			"expected %#v, got %#v",
			map[string]int{
				"red":   1,
				"green": 2,
				"blue":  3,
			},
			s.ColorCodes,
		)
	}

	if s.NestedSpecification.Property != "iamnested" {
		t.Errorf("expected '%s' string, got %#v", "iamnested", s.NestedSpecification.Property)
	}

	if s.NestedSpecification.PropertyWithDefault != "fuzzybydefault" {
		t.Errorf("expected default '%s' string, got %#v", "fuzzybydefault", s.NestedSpecification.PropertyWithDefault)
	}

	if s.AfterNested != "after" {
		t.Errorf("expected default '%s' string, got %#v", "after", s.AfterNested)
	}

	if s.MultiWordVarWithAutoSplit != 24 {
		t.Errorf("expected %q, got %q", 24, s.MultiWordVarWithAutoSplit)
	}

	if s.MultiWordACRWithAutoSplit != 25 {
		t.Errorf("expected %d, got %d", 25, s.MultiWordACRWithAutoSplit)
	}
}

func TestParseErrorBool(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_DEBUG", "string")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")
	err := Process("env_config", &s)
	if err == nil {
		t.Errorf("expected err")
	}
}

func TestParseErrorFloat32(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_RATE", "string")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")
	err := Process("env_config", &s)
	if err == nil {
		t.Errorf("expected err")
	}
}

func TestParseErrorInt(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_PORT", "string")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")
	err := Process("env_config", &s)
	if err == nil {
		t.Errorf("expected err")
	}
}

func TestParseErrorUint(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_TTL", "-30")
	err := Process("env_config", &s)
	if err == nil {
		t.Errorf("expected err")
	}
}

func TestParseErrorSplitWords(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_MULTI_WORD_VAR_WITH_AUTO_SPLIT", "shakespeare")
	err := Process("env_config", &s)
	if err == nil {
		t.Errorf("expected err")
	}
}

func TestUnsetVars(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("USER", "foo")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	if err := config.Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}

	// If the var is not defined the non-prefixed version should not be used
	// unless the struct tag says so
	if s.User != "" {
		t.Errorf("expected %q, got %q", "", s.User)
	}
}

func TestAlternateVarNames(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_MULTI_WORD_VAR", "foo")
	os.Setenv("ENV_CONFIG_MULTI_WORD_VAR_WITH_ALT", "bar")
	os.Setenv("ENV_CONFIG_MULTI_WORD_VAR_WITH_LOWER_CASE_ALT", "baz")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	if err := config.Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}

	// Setting the alt version of the var in the environment has no effect if
	// the struct tag is not supplied
	if s.MultiWordVar != "" {
		t.Errorf("expected %q, got %q", "", s.MultiWordVar)
	}

	// Setting the alt version of the var in the environment correctly sets
	// the value if the struct tag IS supplied
	if s.MultiWordVarWithAlt != "bar" {
		t.Errorf("expected %q, got %q", "bar", s.MultiWordVarWithAlt)
	}

	// Alt value is not case sensitive and is treated as all uppercase
	if s.MultiWordVarWithLowerCaseAlt != "baz" {
		t.Errorf("expected %q, got %q", "baz", s.MultiWordVarWithLowerCaseAlt)
	}
}

func TestRequiredVar(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foobar")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	if err := config.Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}

	if s.RequiredVar != "foobar" {
		t.Errorf("expected %s, got %s", "foobar", s.RequiredVar)
	}
}

func TestRequiredMissing(t *testing.T) {
	var s Specification
	os.Clearenv()

	err := Process("env_config", &s)
	if err == nil {
		t.Error("no failure when missing required variable")
	}
}

func TestBlankDefaultVar(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "requiredvalue")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	if err := config.Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}

	if s.DefaultVar != "foobar" {
		t.Errorf("expected %s, got %s", "foobar", s.DefaultVar)
	}

	if *s.SomePointerWithDefault != "foo2baz" {
		t.Errorf("expected %s, got %s", "foo2baz", *s.SomePointerWithDefault)
	}
}

func TestNonBlankDefaultVar(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_DEFAULTVAR", "nondefaultval")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "requiredvalue")
	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	if err := config.Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}

	if s.DefaultVar != "nondefaultval" {
		t.Errorf("expected %s, got %s", "nondefaultval", s.DefaultVar)
	}
}

func TestRequiredDefault(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	if err := config.Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}

	if s.RequiredDefault != "foo2bar" {
		t.Errorf("expected %q, got %q", "foo2bar", s.RequiredDefault)
	}
}

func TestPointerFieldBlank(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	if err := config.Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}

	if s.SomePointer != nil {
		t.Errorf("expected <nil>, got %q", *s.SomePointer)
	}
}

func TestMustProcess(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_DEBUG", "true")
	os.Setenv("ENV_CONFIG_PORT", "8080")
	os.Setenv("ENV_CONFIG_RATE", "0.5")
	os.Setenv("ENV_CONFIG_USER", "Kelsey")
	os.Setenv("SERVICE_HOST", "127.0.0.1")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	config.MustProcess("env_config", &s)

	defer func() {
		if err := recover(); err != nil {
			return
		}

		t.Error("expected panic")
	}()
	m := make(map[string]string)
	config.MustProcess("env_config", &m)
}

func TestEmbeddedStruct(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "required")
	os.Setenv("ENV_CONFIG_ENABLED", "true")
	os.Setenv("ENV_CONFIG_EMBEDDEDPORT", "1234")
	os.Setenv("ENV_CONFIG_MULTIWORDVAR", "foo")
	os.Setenv("ENV_CONFIG_MULTI_WORD_VAR_WITH_ALT", "bar")
	os.Setenv("ENV_CONFIG_MULTI_WITH_DIFFERENT_ALT", "baz")
	os.Setenv("ENV_CONFIG_EMBEDDED_WITH_ALT", "foobar")
	os.Setenv("ENV_CONFIG_SOMEPOINTER", "foobaz")
	os.Setenv("ENV_CONFIG_EMBEDDED_IGNORED", "was-not-ignored")

	config := NewStructConfig(&Options{
		Tags:      OptionTags{FileTag: "envconfig"},
		FlagNames: OptionFlagNames{Debug: "config-debug"},
	})

	if err := config.Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}

	if !s.Enabled {
		t.Errorf("expected %v, got %v", true, s.Enabled)
	}
	if s.EmbeddedPort != 1234 {
		t.Errorf("expected %d, got %v", 1234, s.EmbeddedPort)
	}
	if s.MultiWordVar != "foo" {
		t.Errorf("expected %s, got %s", "foo", s.MultiWordVar)
	}
	if s.Embedded.MultiWordVar != "foo" {
		t.Errorf("expected %s, got %s", "foo", s.Embedded.MultiWordVar)
	}
	if s.MultiWordVarWithAlt != "bar" {
		t.Errorf("expected %s, got %s", "bar", s.MultiWordVarWithAlt)
	}
	if s.Embedded.MultiWordVarWithAlt != "baz" {
		t.Errorf("expected %s, got %s", "baz", s.Embedded.MultiWordVarWithAlt)
	}
	if s.EmbeddedAlt != "foobar" {
		t.Errorf("expected %s, got %s", "foobar", s.EmbeddedAlt)
	}
	if *s.SomePointer != "foobaz" {
		t.Errorf("expected %s, got %s", "foobaz", *s.SomePointer)
	}
	if s.EmbeddedIgnored != "" {
		t.Errorf("expected empty string, got %#v", s.Ignored)
	}
}

func TestEmbeddedButIgnoredStruct(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "required")
	os.Setenv("ENV_CONFIG_FIRSTEMBEDDEDBUTIGNORED", "was-not-ignored")
	os.Setenv("ENV_CONFIG_SECONDEMBEDDEDBUTIGNORED", "was-not-ignored")
	if err := Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}
	if s.FirstEmbeddedButIgnored != "" {
		t.Errorf("expected empty string, got %#v", s.Ignored)
	}
	if s.SecondEmbeddedButIgnored != "" {
		t.Errorf("expected empty string, got %#v", s.Ignored)
	}
}

func TestNonPointerFailsProperly(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "snap")

	err := Process("env_config", s)
	if err != ErrInvalidSpecification {
		t.Errorf("non-pointer should fail with ErrInvalidSpecification, was instead %s", err)
	}
}

func TestEmptyPrefixUsesFieldNames(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("REQUIREDVAR", "foo")

	err := Process("", &s)
	if err != nil {
		t.Errorf("Process failed: %s", err)
	}

	if s.RequiredVar != "foo" {
		t.Errorf(
			`RequiredVar not populated correctly: expected "foo", got %q`,
			s.RequiredVar,
		)
	}
}

func TestNestedStructVarName(t *testing.T) {
	var s Specification
	os.Clearenv()
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "required")
	val := "found with only short name"
	os.Setenv("INNER", val)
	if err := Process("env_config", &s); err != nil {
		t.Error(err.Error())
	}
	if s.NestedSpecification.Property != val {
		t.Errorf("expected %s, got %s", val, s.NestedSpecification.Property)
	}
}

func BenchmarkGatherInfo(b *testing.B) {
	os.Clearenv()
	os.Setenv("ENV_CONFIG_DEBUG", "true")
	os.Setenv("ENV_CONFIG_PORT", "8080")
	os.Setenv("ENV_CONFIG_RATE", "0.5")
	os.Setenv("ENV_CONFIG_USER", "Kelsey")
	os.Setenv("ENV_CONFIG_TIMEOUT", "2m")
	os.Setenv("ENV_CONFIG_ADMINUSERS", "John,Adam,Will")
	os.Setenv("ENV_CONFIG_MAGICNUMBERS", "5,10,20")
	os.Setenv("ENV_CONFIG_COLORCODES", "red:1,green:2,blue:3")
	os.Setenv("SERVICE_HOST", "127.0.0.1")
	os.Setenv("ENV_CONFIG_TTL", "30")
	os.Setenv("ENV_CONFIG_REQUIREDVAR", "foo")
	os.Setenv("ENV_CONFIG_IGNORED", "was-not-ignored")
	os.Setenv("ENV_CONFIG_OUTER_INNER", "iamnested")
	os.Setenv("ENV_CONFIG_AFTERNESTED", "after")
	os.Setenv("ENV_CONFIG_HONOR", "honor")
	os.Setenv("ENV_CONFIG_DATETIME", "2016-08-16T18:57:05Z")
	os.Setenv("ENV_CONFIG_MULTI_WORD_VAR_WITH_AUTO_SPLIT", "24")

	c := NewStructConfig(nil)

	for i := 0; i < b.N; i++ {
		var s Specification
		c.gatherInfo("", "env_config", &s)
	}
}
