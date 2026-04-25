# Golden File Comparison Test Design

**Date:** 2026-04-25
**Branch:** feat/remove-viper
**Reference branch:** feat-parse-config-to-struct (ground truth)

## Goal

Compare `StructConfig.Process` output between two branch implementations across a comprehensive set of inputs (defaults, env vars, config files, CLI flags). The reference branch (`feat-parse-config-to-struct`) output is serialized as JSON golden files; the current branch tests compare against them.

## Files

| File | Purpose |
|------|---------|
| `structconfig_golden_test.go` | `TestMain`, `goldenScenario`, `goldenResult`, `TestGolden` |
| `testdata/golden/*.json` | One JSON file per scenario, committed from reference branch |
| `testdata/golden/config.toml` | TOML config for file-based scenarios |
| `testdata/golden/config.yaml` | YAML config for file-based scenarios |

No changes to `structconfig_test.go` or production code.

## Workflow

```bash
# Step 1: on feat-parse-config-to-struct
go test -run TestGolden -update    # generates testdata/golden/*.json
git add testdata/golden/ && git commit -m "chore: add golden files from reference branch"

# Step 2: copy structconfig_golden_test.go to feat/remove-viper
# (cherry-pick or manual copy)

# Step 3: on feat/remove-viper
go test -run TestGolden            # fails where implementations diverge
```

## Data Structures

```go
type goldenScenario struct {
    name       string
    envVars    map[string]string  // applied with t.Setenv (auto-cleaned)
    configFile string             // path appended as --config <path>
    args       []string           // replaces os.Args[1:] for the test
    wantErr    bool
}

type goldenResult struct {
    Spec  Specification `json:"spec"`
    Error string        `json:"error,omitempty"`
}
```

## Subtest Flow

For each scenario `t.Run(sc.name, ...)`:

1. Set env vars via `t.Setenv` (auto-restored on cleanup)
2. Save `os.Args`, set `os.Args = append([]string{"test"}, sc.args...)` with `t.Cleanup` to restore
3. If `configFile` non-empty, append `--config <path>` to `os.Args`
4. Create `NewStructConfig` with `FileTag: "envconfig"`, `FlagNames.Debug: "config-debug"` (matches existing tests)
5. Call `config.Process("env_config", &s)`
6. Build `goldenResult{Spec: s, Error: errStr}`
7. **`-update` path**: write `testdata/golden/<name>.json` with `json.MarshalIndent`
8. **Normal path**: read file, unmarshal, compare with `reflect.DeepEqual`; on mismatch log `%+v` of both

## Scenario Table

| # | Name | Env | Config File | CLI Flags | Tests |
|---|------|-----|-------------|-----------|-------|
| 1 | `defaults_only` | — | — | — | struct tag defaults |
| 2 | `env_full` | all fields | — | — | full env coverage |
| 3 | `env_split_words` | `MULTI_WORD_VAR_*` | — | — | `split_words` → UPPER_SNAKE_CASE |
| 4 | `env_alt_name` | `SERVICE_HOST`, `BROKER` | — | — | `env`/`envconfig` tag overrides |
| 5 | `config_toml` | — | toml | — | TOML file → struct |
| 6 | `config_yaml` | — | yaml | — | YAML file → struct |
| 7 | `env_overrides_file` | some fields | toml | — | env wins over file |
| 8 | `flags_basic` | — | — | `--port 9090 --user alice` | flags populate struct |
| 9 | `flags_override_env` | port=8080 | — | `--port 9090` | flag wins over env |
| 10 | `flags_override_file` | — | toml | `--port 9090` | flag wins over file |
| 11 | `flags_override_all` | some fields | toml | `--port 9090` | flag is highest priority |
| 12 | `nested_struct` | `ENV_CONFIG_OUTER_*` | — | — | nested struct fields |
| 13 | `embedded_struct` | embedded env vars | — | — | embedded struct squash |
| 14 | `pointer_fields` | — | — | — | nil vs. defaulted pointer |
| 15 | `required_missing` | — | — | — | Process returns non-nil error |

## Config Testdata Files

`testdata/golden/config.toml` and `config.yaml` set a recognizable subset of fields to values that differ from env var values, so priority scenarios (7, 10, 11) produce unambiguous signal about which source won.

Example (TOML):
```toml
port = 7777
user = "from-file"
requiredvar = "file-required"

[outer]
inner = "file-nested"
```

## Notes

- `os.Args` override is not goroutine-safe; subtests using `args` or `configFile` must not use `t.Parallel()`
- `required_missing` scenario sets no env vars and no defaults for `RequiredVar` — verifies error propagation
- No new dependencies; uses only `encoding/json` and `reflect` from stdlib
