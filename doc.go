// Package structconfig populates a struct from multiple configuration sources.
//
// Source precedence is:
// defaults < config file < environment variables < CLI flags.
//
// The package is app-oriented and is intended for startup-time configuration
// loading. A StructConfig value is expected to be initialized and processed once
// during application startup.
//
// MustProcess prints any output returned by Process. When built-in control-flow
// flags are used (--version, --default-config, --debug), MustProcess exits with
// status code 0. For all other errors, MustProcess panics.
package structconfig
