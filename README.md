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

	if err := structconfig.Process("myapp", &cfg); err != nil {
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
err := structconfig.Process("myapp", &cfg)
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
		FileTag: "mapstructure",
	},
	FlagNames: structconfig.OptionFlagNames{
		Debug: "config-debug",
	},
})

err := config.Process("myapp", &cfg)
```

## Struct Tags

`structconfig` reads these tags:

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
| `--config`, `-c` | Path to a config file. |
| `--config-type`, `-t` | Config file format, `toml` or `yaml`. |
| `--default-config`, `-p` | Print a config file containing defaults and zero values, then exit. |
| `--version`, `-V` | Print the string from `VersionFunc`, then exit. |
| `--debug`, `-d` | Print config debug info and exit. The long flag name is customizable through `Options.FlagNames.Debug`. |

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
- `MustProcess` panics on error.
- Missing config files are treated as non-fatal when a path is supplied; decoding continues with the remaining sources.
