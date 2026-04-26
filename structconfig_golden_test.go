//go:build golden

package structconfig_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/justakit/structconfig"
)

var update = flag.Bool("update", false, "update golden files")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

type goldenScenario struct {
	name       string
	envVars    map[string]string
	configFile string
	args       []string
}

type goldenResult struct {
	Spec  Specification `json:"spec"`
	Error string        `json:"error,omitempty"`
}

var scenarios = []goldenScenario{
	{
		name: "defaults_only",
		envVars: map[string]string{
			"ENV_CONFIG_REQUIREDVAR": "req",
		},
	},
	{
		name: "env_full",
		envVars: map[string]string{
			"ENV_CONFIG_DEBUG":                          "true",
			"ENV_CONFIG_PORT":                           "8080",
			"ENV_CONFIG_RATE":                           "0.5",
			"ENV_CONFIG_USER":                           "Kelsey",
			"ENV_CONFIG_TIMEOUT":                        "2m",
			"ENV_CONFIG_ADMINUSERS":                     "John,Adam,Will",
			"ENV_CONFIG_MAGICNUMBERS":                   "5,10,20",
			"ENV_CONFIG_EMPTYNUMBERS":                   "",
			"ENV_CONFIG_COLORCODES":                     "red=1,green=2,blue=3",
			"SERVICE_HOST":                              "127.0.0.1",
			"ENV_CONFIG_TTL":                            "30",
			"ENV_CONFIG_REQUIREDVAR":                    "foo",
			"ENV_CONFIG_OUTER_INNER":                    "iamnested",
			"ENV_CONFIG_AFTERNESTED":                    "after",
			"ENV_CONFIG_MULTI_WORD_VAR_WITH_AUTO_SPLIT": "24",
			"ENV_CONFIG_MULTI_WORD_ACR_WITH_AUTO_SPLIT": "25",
			"ENV_CONFIG_MULTIWORDVAR":                   "multi",
			"ENV_CONFIG_MAPFIELD":                       "alpha=beta,gamma=delta",
		},
	},
	{
		name: "env_split_words",
		envVars: map[string]string{
			"ENV_CONFIG_REQUIREDVAR":                    "req",
			"ENV_CONFIG_MULTI_WORD_VAR_WITH_AUTO_SPLIT": "24",
			"ENV_CONFIG_MULTI_WORD_ACR_WITH_AUTO_SPLIT": "25",
		},
	},
	{
		name: "env_alt_name",
		envVars: map[string]string{
			"ENV_CONFIG_REQUIREDVAR": "req",
			"SERVICE_HOST":           "1.2.3.4",
			"ENV_CONFIG_BROKER":      "mybroker",
		},
	},
	{
		name:       "config_toml",
		configFile: "testdata/golden/config.toml",
	},
	{
		name:       "config_yaml",
		configFile: "testdata/golden/config.yaml",
	},
	{
		name:       "env_overrides_file",
		configFile: "testdata/golden/config.toml",
		envVars: map[string]string{
			"ENV_CONFIG_PORT": "9999",
			"ENV_CONFIG_USER": "env-user",
		},
	},
	{
		name: "flags_basic",
		args: []string{
			"--port", "9090",
			"--user", "alice",
			"--requiredvar", "flagreq",
		},
	},
	{
		name: "flags_override_env",
		envVars: map[string]string{
			"ENV_CONFIG_PORT":        "8080",
			"ENV_CONFIG_REQUIREDVAR": "foo",
		},
		args: []string{"--port", "9090"},
	},
	{
		name:       "flags_override_file",
		configFile: "testdata/golden/config.toml",
		args:       []string{"--port", "9090"},
	},
	{
		name:       "flags_override_all",
		configFile: "testdata/golden/config.toml",
		envVars: map[string]string{
			"ENV_CONFIG_USER": "env-user",
		},
		args: []string{"--port", "9090"},
	},
	{
		name: "nested_struct",
		envVars: map[string]string{
			"ENV_CONFIG_REQUIREDVAR":       "req",
			"ENV_CONFIG_OUTER_INNER":       "nested-val",
			"ENV_CONFIG_OUTER_INTPROPERTY": "7",
		},
	},
	{
		name: "embedded_struct",
		envVars: map[string]string{
			"ENV_CONFIG_REQUIREDVAR":        "req",
			"ENV_CONFIG_ENABLED":            "true",
			"ENV_CONFIG_EMBEDDEDPORT":       "5050",
			"ENV_CONFIG_MULTIWORDVARNESTED": "emb-multi",
			"ENV_CONFIG_EMBEDDED_WITH_ALT":  "emb-alt",
		},
	},
	{
		name: "pointer_fields",
		envVars: map[string]string{
			"ENV_CONFIG_REQUIREDVAR": "req",
			"ENV_CONFIG_SOMEPOINTER": "set-ptr",
		},
	},
	{
		name: "required_missing",
	},
}

func TestGolden(t *testing.T) {
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range sc.envVars {
				t.Setenv(k, v)
			}

			origArgs := os.Args
			t.Cleanup(func() { os.Args = origArgs })
			newArgs := append([]string{"test"}, sc.args...)
			if sc.configFile != "" {
				newArgs = append(newArgs, "--config", sc.configFile)
			}
			os.Args = newArgs

			var s Specification
			config := structconfig.NewStructConfig(&structconfig.Options{
				Tags:      structconfig.OptionTags{FileTag: "envconfig"},
				FlagNames: structconfig.OptionFlagNames{Debug: "config-debug"},
			})
			_, err := config.Process("env_config", &s)

			errStr := ""
			if err != nil {
				errStr = err.Error()
			}

			got := goldenResult{Spec: s, Error: errStr}
			goldenPath := filepath.Join("testdata", "golden", sc.name+".json")

			if *update {
				data, err := json.MarshalIndent(got, "", "  ")
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
					t.Fatalf("mkdir golden: %v", err)
				}
				if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}

			data, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden file missing: %s (run: go test -run TestGolden -update)", goldenPath)
			}
			var want goldenResult
			if err := json.Unmarshal(data, &want); err != nil {
				t.Fatalf("unmarshal golden: %v", err)
			}

			compareGolden(t, sc.name, got, want)
		})
	}
}

func compareGolden(t *testing.T, name string, got, want goldenResult) {
	t.Helper()
	if got.Error != want.Error {
		t.Errorf("%s: error mismatch\n  got:  %q\n  want: %q", name, got.Error, want.Error)
		return
	}
	if got.Error != "" {
		return
	}
	gotJSON, _ := json.MarshalIndent(got.Spec, "", "  ")
	wantJSON, _ := json.MarshalIndent(want.Spec, "", "  ")
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("%s: spec mismatch\n  got:\n%s\n  want:\n%s", name, gotJSON, wantJSON)
	}
}
