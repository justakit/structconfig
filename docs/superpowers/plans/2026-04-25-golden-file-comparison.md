# Golden File Comparison Test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a table-driven `TestGolden` that captures `StructConfig.Process` outputs as JSON golden files from the reference branch (`feat-parse-config-to-struct`) and then compares them against `feat/remove-viper` on every `go test` run.

**Architecture:** A new `structconfig_golden_test.go` file adds `TestMain` (registers `-update` flag), a `goldenScenario` table of 15 input combinations (env vars, config file, CLI flags, defaults), and `TestGolden` which serializes results to `testdata/golden/<name>.json` when `-update` is set or compares against them otherwise. Config testdata files live in `testdata/golden/`. Golden JSON files are generated on the reference branch using a git worktree, then committed to the current branch.

**Tech Stack:** Go stdlib only — `encoding/json`, `reflect`, `os`, `path/filepath`; `github.com/spf13/pflag` (already a dependency); standard `go test -update` pattern.

---

### Task 1: Write golden config testdata files

**Files:**
- Create: `testdata/golden/config.toml`
- Create: `testdata/golden/config.yaml`

These files are used by the `config_toml`, `config_yaml`, `env_overrides_file`, `flags_override_file`, and `flags_override_all` scenarios. Keys use mapstructure field names (no env_config prefix). Values are deliberately distinct from env var values so priority tests produce unambiguous signal.

- [ ] **Step 1: Create testdata/golden/ directory and TOML file**

```bash
mkdir -p testdata/golden
```

Write `testdata/golden/config.toml`:
```toml
port = 7777
user = "from-file"
requiredvar = "file-required"
afternested = "file-after"
rate = 3.14
ttl = 99

[outer]
inner = "file-nested"
intproperty = 42
```

- [ ] **Step 2: Write YAML file**

Write `testdata/golden/config.yaml`:
```yaml
port: 7777
user: "from-file"
requiredvar: "file-required"
afternested: "file-after"
rate: 3.14
ttl: 99

outer:
  inner: "file-nested"
  intproperty: 42
```

- [ ] **Step 3: Verify both files parse without error**

```bash
go run -e - <<'EOF'
package main
import (
    "fmt"
    "os"
    toml "github.com/pelletier/go-toml/v2"
    "gopkg.in/yaml.v3"
)
func main() {
    var m map[string]any
    b, _ := os.ReadFile("testdata/golden/config.toml")
    if err := toml.Unmarshal(b, &m); err != nil { fmt.Println("TOML error:", err); return }
    fmt.Println("TOML ok:", m)
    b, _ = os.ReadFile("testdata/golden/config.yaml")
    if err := yaml.Unmarshal(b, &m); err != nil { fmt.Println("YAML error:", err); return }
    fmt.Println("YAML ok:", m)
}
EOF
```
Expected: both print "ok: map[...]" with no error lines.

- [ ] **Step 4: Commit**

```bash
git add testdata/golden/config.toml testdata/golden/config.yaml
git commit -m "test: add golden config testdata files for comparison scenarios"
```

---

### Task 2: Write structconfig_golden_test.go

**Files:**
- Create: `structconfig_golden_test.go`

This file must compile on both branches since it will be used on `feat-parse-config-to-struct` to generate golden files and on `feat/remove-viper` to compare.

- [ ] **Step 1: Write the file**

Write `structconfig_golden_test.go`:

```go
package structconfig_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
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
	wantErr    bool
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
			"ENV_CONFIG_DEBUG":                        "true",
			"ENV_CONFIG_PORT":                         "8080",
			"ENV_CONFIG_RATE":                         "0.5",
			"ENV_CONFIG_USER":                         "Kelsey",
			"ENV_CONFIG_TIMEOUT":                      "2m",
			"ENV_CONFIG_ADMINUSERS":                   "John,Adam,Will",
			"ENV_CONFIG_MAGICNUMBERS":                 "5,10,20",
			"ENV_CONFIG_EMPTYNUMBERS":                 "",
			"ENV_CONFIG_COLORCODES":                   "red=1,green=2,blue=3",
			"SERVICE_HOST":                            "127.0.0.1",
			"ENV_CONFIG_TTL":                          "30",
			"ENV_CONFIG_REQUIREDVAR":                  "foo",
			"ENV_CONFIG_OUTER_INNER":                  "iamnested",
			"ENV_CONFIG_AFTERNESTED":                  "after",
			"ENV_CONFIG_MULTI_WORD_VAR_WITH_AUTO_SPLIT": "24",
			"ENV_CONFIG_MULTI_WORD_ACR_WITH_AUTO_SPLIT": "25",
			"ENV_CONFIG_MULTIWORDVAR":                 "multi",
			"ENV_CONFIG_MAPFIELD":                     "alpha=beta,gamma=delta",
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
			"ENV_CONFIG_REQUIREDVAR":      "req",
			"ENV_CONFIG_OUTER_INNER":      "nested-val",
			"ENV_CONFIG_OUTER_INTPROPERTY": "7",
		},
	},
	{
		name: "embedded_struct",
		envVars: map[string]string{
			"ENV_CONFIG_REQUIREDVAR":      "req",
			"ENV_CONFIG_ENABLED":          "true",
			"ENV_CONFIG_EMBEDDEDPORT":     "5050",
			"ENV_CONFIG_MULTIWORDVARNESTED": "emb-multi",
			"ENV_CONFIG_EMBEDDED_WITH_ALT":  "emb-alt",
		},
	},
	{
		name: "pointer_fields",
		envVars: map[string]string{
			"ENV_CONFIG_REQUIREDVAR":  "req",
			"ENV_CONFIG_SOMEPOINTER":  "set-ptr",
		},
	},
	{
		name:    "required_missing",
		wantErr: true,
	},
}

func TestGolden(t *testing.T) {
	for _, sc := range scenarios {
		sc := sc
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
			err := config.Process("env_config", &s)

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
	gotHasErr := got.Error != ""
	wantHasErr := want.Error != ""
	if gotHasErr != wantHasErr {
		t.Errorf("%s: error presence mismatch\n  got error:  %q\n  want error: %q", name, got.Error, want.Error)
		return
	}
	if gotHasErr {
		return
	}
	if !reflect.DeepEqual(got.Spec, want.Spec) {
		gotJSON, _ := json.MarshalIndent(got.Spec, "  ", "  ")
		wantJSON, _ := json.MarshalIndent(want.Spec, "  ", "  ")
		t.Errorf("%s: spec mismatch\n  got:\n  %s\n  want:\n  %s", name, gotJSON, wantJSON)
	}
}
```

- [ ] **Step 2: Verify it compiles on the current branch**

```bash
go build ./...
go vet ./...
```
Expected: no errors.

- [ ] **Step 3: Verify TestGolden fails gracefully without golden files**

```bash
go test -run TestGolden -v 2>&1 | head -40
```
Expected: each subtest fails with `golden file missing: testdata/golden/<name>.json (run: go test -run TestGolden -update)`.

- [ ] **Step 4: Commit**

```bash
git add structconfig_golden_test.go
git commit -m "test: add TestGolden table-driven golden file test"
```

---

### Task 3: Generate golden JSON files on the reference branch

**Files:**
- Create: `testdata/golden/*.json` (15 files, from reference branch)

Uses `git worktree` so we don't need to switch branches.

- [ ] **Step 1: Create a worktree for the reference branch**

```bash
git worktree add /tmp/sc-ref feat-parse-config-to-struct
```
Expected: `Preparing worktree (checking out 'feat-parse-config-to-struct')` — no errors.

- [ ] **Step 2: Copy the golden test file and testdata into the worktree**

```bash
cp structconfig_golden_test.go /tmp/sc-ref/
mkdir -p /tmp/sc-ref/testdata/golden
cp testdata/golden/config.toml /tmp/sc-ref/testdata/golden/
cp testdata/golden/config.yaml /tmp/sc-ref/testdata/golden/
```

- [ ] **Step 3: Verify test file compiles in the worktree**

```bash
cd /tmp/sc-ref && go build ./... && go vet ./...
```
Expected: no errors. If there are compile errors due to API differences between branches, resolve them in the copy at `/tmp/sc-ref/structconfig_golden_test.go` (do NOT edit the original).

- [ ] **Step 4: Generate golden files**

```bash
cd /tmp/sc-ref && go test -run TestGolden -update -v
```
Expected: 15 subtests all pass (they write files and return). Output like:
```
--- PASS: TestGolden/defaults_only (0.00s)
--- PASS: TestGolden/env_full (0.00s)
...
--- PASS: TestGolden/required_missing (0.00s)
PASS
```

If any subtest fails with an unexpected error (not "golden file missing"), investigate:
- Check if the reference branch `Process` API signature differs
- Check if any env var name assumptions are wrong (run `go test -run TestProcess -v` in worktree to see what works)

- [ ] **Step 5: Copy generated JSON files back**

```bash
cp /tmp/sc-ref/testdata/golden/*.json testdata/golden/
```
Verify 15 JSON files were created:
```bash
ls testdata/golden/*.json | wc -l
```
Expected: `15`

- [ ] **Step 6: Inspect a few golden files for sanity**

```bash
cat testdata/golden/defaults_only.json
cat testdata/golden/env_full.json
cat testdata/golden/required_missing.json
```
Expected:
- `defaults_only.json`: Port=0, DefaultVar="foobar", SomePointerWithDefault="foo2baz", NoPrefixDefault="127.0.0.1", RequiredDefault="foo2bar", RequiredVar="req", MapField={"one":"two","three":"four"}, NestedSpecification.PropertyWithDefault="fuzzybydefault"
- `env_full.json`: Port=8080, User="Kelsey", Debug=true, Timeout=120000000000 (2m in ns), AdminUsers=["John","Adam","Will"]
- `required_missing.json`: Error field non-empty, Spec is zero value

- [ ] **Step 7: Remove the worktree**

```bash
cd /Users/vkit/go/src/github.com/justakit/structconfig
git worktree remove /tmp/sc-ref
```

- [ ] **Step 8: Commit the golden files**

```bash
git add testdata/golden/*.json
git commit -m "test: add golden JSON files generated from reference branch"
```

---

### Task 4: Run TestGolden on feat/remove-viper and analyze failures

- [ ] **Step 1: Run the full golden test suite**

```bash
go test -run TestGolden -v 2>&1 | tee /tmp/golden-results.txt
```
Expected: some subtests pass, some fail with spec mismatch diffs showing divergence between the two implementations.

- [ ] **Step 2: Summarize passing vs failing scenarios**

```bash
grep -E "^--- (PASS|FAIL): TestGolden" /tmp/golden-results.txt
```
This gives you the table of which inputs reveal implementation differences.

- [ ] **Step 3: Run the full test suite to confirm no regressions**

```bash
go test ./...
```
Expected: all pre-existing tests still pass (golden failures are in addition to, not instead of, existing tests).

- [ ] **Step 4: Commit final state**

```bash
git add .
git commit -m "test: run golden comparison — documents divergence from reference branch"
```

---

## Reference: Key derivation for Specification with prefix env_config

Both branches call `gatherInfo("", prefix, spec)` — the key prefix is **empty**, so flag names are short:

| Field | Flag name | Env var |
|-------|-----------|---------|
| `Port` | `port` | `ENV_CONFIG_PORT` |
| `User` | `user` | `ENV_CONFIG_USER` |
| `RequiredVar` | `requiredvar` | `ENV_CONFIG_REQUIREDVAR` |
| `AfterNested` | `afternested` | `ENV_CONFIG_AFTERNESTED` |
| `MultiWordVarWithAutoSplit` (split_words) | `multiwordvarwithautosplit` | `ENV_CONFIG_MULTI_WORD_VAR_WITH_AUTO_SPLIT` |
| `MultiWordVarWithAlt` (envconfig:"MULTI_WORD_VAR_WITH_ALT") | `multi_word_var_with_alt` | `ENV_CONFIG_MULTI_WORD_VAR_WITH_ALT` |
| `NoPrefixWithAlt` (env:"SERVICE_HOST") | `noprefixwithalt` | `SERVICE_HOST` |
| `NoPrefixDefault` (envconfig:"BROKER") | `broker` | `ENV_CONFIG_BROKER` |
| `NestedSpecification.Property` (outer→inner) | `outer-inner` | `ENV_CONFIG_OUTER_INNER` |

`NoPrefixDefault` has `envconfig:"BROKER"` which overrides the Name, so Env = `strings.ToUpper("env_config_BROKER")` = `ENV_CONFIG_BROKER`. Its default is "127.0.0.1" so it always has a value.

## Config file key mapping (TOML/YAML — no prefix)

| Config key | Maps to field | Notes |
|------------|---------------|-------|
| `port` | `Port` | lowercase field name |
| `user` | `User` | |
| `requiredvar` | `RequiredVar` | |
| `afternested` | `AfterNested` | |
| `[outer] / inner` | `NestedSpecification.Property` | envconfig:"outer" → "inner" |
| `[outer] / intproperty` | `NestedSpecification.IntProperty` | |
| `rate` | `Rate` | float32 |
| `ttl` | `TTL` | uint32 |
