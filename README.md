# structconfig

```go
import "github.com/justakit/structconfig"
```

`structconfig` populates a struct from four configuration sources in a fixed order:

1. Struct tag defaults
2. Config file values
3. Environment variables
4. CLI flags

Higher-priority sources override lower-priority ones.

The package started from the `envconfig` model, but the current library is broader: it can merge defaults, TOML or YAML config files, environment variables, and command-line flags into one config struct.

## Documentation

See [pkg.go.dev](https://pkg.go.dev/github.com/justakit/structconfig).

## Quick Start

Given this config file:

```toml
debug = true
port = 8080
user = "file-user"
timeout = "45s"
adminusers = ["alice", "bob"]
colorcodes = { red = 1, blue = 2 }

[database]
host = "db.internal"
```

and these environment variables:

```bash
export MYAPP_USER=env-user
export MYAPP_COLORCODES=red=10,green=20
```

you can load configuration like this:

```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/justakit/structconfig"
)

type Config struct {
	Debug      bool           `default:"false"`
	Port       int            `default:"3000"`
	User       string         `required:"true"`
	Timeout    time.Duration  `default:"30s"`
	AdminUsers []string
	ColorCodes map[string]int `default:"red=1,blue=2"`
	Database   struct {
		Host string `default:"localhost"`
	}
}

func main() {
	var cfg Config

	if _, err := structconfig.Process("myapp", &cfg); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%+v\n", cfg)
}
```

Run it with an explicit config file and an overriding flag:

```bash
myapp --config ./config.toml --port 9090
```

In that example:

- `debug` comes from the file
- `user` comes from `MYAPP_USER`
- `port` comes from `--port`
- any field left unset falls back to its `default` tag or the zero value

## API

Use the package-level helpers when the defaults are enough:

```go
out, err := structconfig.Process("myapp", &cfg)
structconfig.MustProcess("myapp", &cfg)
```

Use `NewStructConfig` when you need custom options:

```go
config := structconfig.NewStructConfig(&structconfig.Options{
	ConfigType: "yaml",
	VersionFunc: func() string {
		return "myapp 1.2.3"
	},
	Tags: structconfig.OptionTags{
		FileTag:  "mapstructure",
		FlagTag:  "cli",
		ShortTag: "alias",
		EnvTag:   "envvar",
		DescTag:  "help",
	},
	FlagNames: structconfig.OptionFlagNames{
		ConfigPath:    "conf",
		ConfigType:    "type",
		DefaultConfig: "show-defaults",
		Version:       "ver",
		Debug:         "cfg-debug",
	},
	FlagShorts: structconfig.OptionFlagShorts{
		ConfigPath:    "C",
		ConfigType:    "T",
		DefaultConfig: "D",
		Version:       "v",
		Debug:         "g",
	},
})

out, err := config.Process("myapp", &cfg)
```

`Process` returns `(string, error)`:

- normal success: `"", nil`
- `--version`: version text and `ErrVersionCalled`
- `--default-config`: encoded config text and `ErrDefaultConfigCalled`
- `--debug`: encoded merged config text (all sources applied) and `ErrDebugCalled`
This package does not call `os.Exit`; callers decide whether to print output and exit.

`Options.Tags` lets you rename the struct tags used by `structconfig`:

| Field | Default Tag | Controls |
| --- | --- | --- |
| `FileTag` | `file` | Config-file key tag and mapstructure tag used during decode. |
| `FlagTag` | `flag` | CLI flag name override tag. |
| `ShortTag` | `short` | CLI shorthand alias tag. |
| `EnvTag` | `env` | Environment variable override tag. |
| `DescTag` | `desc` | Extra help-text/description tag appended to flag usage. |

`Options.FlagNames` lets you customize the long names of built-in flags:

| Field | Default | Controls |
| --- | --- | --- |
| `ConfigPath` | `config` | `--config` flag name. |
| `ConfigType` | `config-type` | `--config-type` flag name. |
| `DefaultConfig` | `default-config` | `--default-config` flag name. |
| `Version` | `version` | `--version` flag name. |
| `Debug` | `debug` | Debug flag name (used for config output). |

`Options.FlagShorts` lets you customize the short aliases for built-in flags:

| Field | Default | Controls |
| --- | --- | --- |
| `ConfigPath` | `c` | `-c` shorthand. |
| `ConfigType` | `t` | `-t` shorthand. |
| `DefaultConfig` | `p` | `-p` shorthand. |
| `Version` | `V` | `-V` shorthand. |
| `Debug` | `d` | `-d` shorthand. |

## Struct Tags

`structconfig` reads these tags (names are configurable via `Options.Tags` for `file`, `flag`, `short`, `env`, and `desc`):

| Tag | Meaning |
| --- | --- |
| `env` | Override the environment variable name for a field. Use `"-"` to disable env binding. |
| `flag` | Override the generated CLI flag name. Use `"-"` to disable the flag. |
| `short` | Define a one-letter shorthand flag alias. Use `"-"` to disable shorthand. |
| `file` | Override the config file key for a field. This tag name is configurable through `Options.Tags.FileTag`. |
| `default` | Default value used when no higher-priority source provides a value. |
| `required` | Mark the field as required. Missing values return an error. |
| `desc` | Extra description appended to the generated flag help text. |
| `ignored` | Skip the field entirely. |
| `split_words` | Split CamelCase field names into `UPPER_SNAKE_CASE` for env lookup. |

Examples:

```go
type Config struct {
	Port        int    `default:"8080" short:"p"`
	ServiceHost string `env:"SERVICE_HOST"`
	LogLevel    string `flag:"log-level" file:"log.level"`
	APIKey      string `required:"true" split_words:"true"`
	Secret      string `ignored:"true"`
}
```

### Naming Rules

- Environment variable names default to `PREFIX_FIELDNAME` in uppercase.
- With `split_words:"true"`, `AutoSplitVar` becomes `PREFIX_AUTO_SPLIT_VAR`.
- Config file keys default to the field name, lowercased.
- Nested struct fields use dot-separated keys internally, and generated flags replace dots with dashes.
- Anonymous embedded structs are flattened into the parent scope.

## Config Files

Config files are only read when `--config` is provided.

Supported formats:

- TOML
- YAML

The config type defaults to `toml` and can be changed by either:

- `Options.ConfigType`
- `--config-type toml|yaml`

Example:

```bash
myapp --config ./config.yaml --config-type yaml
```

## Built-In Flags

Every `Process` call registers these built-in flags in addition to the flags derived from your struct:

| Flag | Meaning |
| --- | --- | 
| `--config`, `-c` | Path to a config file. Both long and short names are customizable via `Options.FlagNames.ConfigPath` and `Options.FlagShorts.ConfigPath`. |
| `--config-type`, `-t` | Config file format, `toml` or `yaml`. Both long and short names are customizable via `Options.FlagNames.ConfigType` and `Options.FlagShorts.ConfigType`. |
| `--default-config`, `-p` | Returns a config string containing defaults and zero values through `Process` output with `ErrDefaultConfigCalled`. Both long and short names are customizable via `Options.FlagNames.DefaultConfig` and `Options.FlagShorts.DefaultConfig`. |
| `--version`, `-V` | Returns the string from `VersionFunc` through `Process` output with `ErrVersionCalled`. Both long and short names are customizable via `Options.FlagNames.Version` and `Options.FlagShorts.Version`. |
| `--debug`, `-d` | Returns the fully merged config (defaults → file → env → flags) as an encoded string through `Process` output with `ErrDebugCalled`. Both long and short names are customizable via `Options.FlagNames.Debug` and `Options.FlagShorts.Debug`. |

## Supported Field Types

The current implementation supports these field types when decoding into the target struct:

- `string`
- `bool`
- `int`, `int8`, `int16`, `int32`, `int64`
- `uint`, `uint8`, `uint16`, `uint32`, `uint64`
- `float32`, `float64`
- `time.Duration`
- slices of supported scalar types
- `map[string]string`
- `map[string]int`
- `map[string]int64`
- pointers to supported types
- nested and embedded structs

CLI flag registration is narrower than file and env decoding for maps: only `map[string]string`, `map[string]int`, and `map[string]int64` are supported as flags.

## Defaults, Required Values, and Zero Values

- `default` tags are applied first.
- A config file overrides defaults.
- Environment variables override the config file.
- CLI flags override everything else.
- `required:"true"` checks whether any source provided a value for the field.
- If no source provides a value and no `default` tag is present, the field keeps its Go zero value.

## Notes

- The package expects a pointer to a struct. Passing anything else returns `ErrInvalidSpecification`.
- `MustProcess` prints any non-empty output returned by `Process`, then panics on error.
- Missing config files are treated as non-fatal when a path is supplied; decoding continues with the remaining sources.
