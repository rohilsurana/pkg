# configs

Unified configuration loading for Go. Define a struct with tags, call `Load`, and get values merged from defaults, YAML files, environment variables, and command-line flags — in that precedence order.

```
flags > env vars > config file > defaults
```

## Install

```bash
go get github.com/rohilsurana/pkg/configs
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "os"
    "time"

    "github.com/rohilsurana/pkg/configs"
)

type Config struct {
    Port     int           `yaml:"port" default:"8080" description:"server port" validate:"min=1,max=65535"`
    Host     string        `yaml:"host" default:"0.0.0.0"`
    LogLevel string        `yaml:"log_level" default:"info" validate:"oneof=debug info warn error"`
    Timeout  time.Duration `yaml:"timeout" default:"30s"`
    Database struct {
        Host     string `yaml:"host" default:"localhost"`
        Port     int    `yaml:"port" default:"5432"`
        Name     string `yaml:"name" required:"true"`
        Password string `yaml:"password" sensitive:"true"`
    } `yaml:"database"`
}

func main() {
    cfg := &Config{}
    err := configs.Load(cfg,
        configs.WithConfigFile("config.yaml"),
        configs.WithEnvPrefix("APP"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Listening on %s:%d\n", cfg.Host, cfg.Port)
}
```

```yaml
# config.yaml
port: 9090
host: 0.0.0.0
database:
  host: db.prod.internal
  port: 5432
  name: myapp
  password: s3cret
```

```bash
# Override via env
export APP_DATABASE_PASSWORD=new-secret

# Override via flags
./myapp --port 3000 --log-level debug
```

## Struct Tags

| Tag | Purpose | Example |
|-----|---------|---------|
| `yaml` | YAML key name; base for deriving env/flag names | `yaml:"log_level"` |
| `default` | Default value when no source provides one | `default:"8080"` |
| `required` | Error if no source provides a value and no default | `required:"true"` |
| `env` | Override the auto-derived env var **leaf** name | `env:"DB_PASS"` |
| `flag` | Override the auto-derived flag **leaf** name | `flag:"listen-port"` |
| `description` | Help text shown for the flag | `description:"server port"` |
| `sensitive` | Mask value in `Print` output | `sensitive:"true"` |
| `validate` | Validation rules | `validate:"min=1,max=65535"` |

If no `yaml` tag is present, the field name is converted to `snake_case` automatically.

## Name Derivation

Names are derived from the `yaml` tag (or snake_cased field name), joined with the parent struct's path:

| Source | Format | Example |
|--------|--------|---------|
| YAML | nested structure | `database.host` |
| Env var | `_` separated, uppercased | `DATABASE_HOST` |
| Flag | `-` separated | `--database-host` |

The `env` and `flag` tags only override the **leaf** segment — the parent prefix is always preserved:

```go
type Config struct {
    Database struct {
        Password string `yaml:"password" env:"SECRET" flag:"secret"`
    } `yaml:"database"`
}
// Env var:  DATABASE_SECRET  (not just SECRET)
// Flag:    --database-secret (not just --secret)
```

With `WithEnvPrefix("APP")`, the env var becomes `APP_DATABASE_SECRET`.

Single-character `flag` tags become shorthands:

```go
Verbose bool `yaml:"verbose" flag:"v" default:"false"`
// Both --verbose and -v work
```

## Options

```go
configs.WithConfigFile("config.yaml") // path to YAML config file
configs.WithEnvPrefix("APP")          // prefix for env vars: APP_PORT, APP_HOST, etc.
configs.WithArgs([]string{...})       // override os.Args[1:] for flag parsing (useful for testing)
configs.WithoutFlags()                // disable flag parsing entirely
```

## Validation

Add a `validate` tag with comma-separated rules:

```go
type Config struct {
    Port     int      `yaml:"port" default:"8080" validate:"min=1,max=65535"`
    LogLevel string   `yaml:"log_level" default:"info" validate:"oneof=debug info warn error"`
    Token    string   `yaml:"token" validate:"notempty"`
    Name     string   `yaml:"name" validate:"min=3,max=50"`
    Tags     []string `yaml:"tags" validate:"max=10"`
}
```

| Rule | Applies to | Description |
|------|-----------|-------------|
| `min=N` | numbers: value >= N; strings/slices: length >= N | Minimum bound |
| `max=N` | numbers: value <= N; strings/slices: length <= N | Maximum bound |
| `oneof=a b c` | any (compared as string) | Value must be one of the space-separated options |
| `notempty` | strings, slices, maps | Must not be empty |

Validation runs after all sources are merged. On failure, `Load` returns a `*configs.ValidationError` with per-field details:

```go
err := configs.Load(cfg, configs.WithoutFlags())
var ve *configs.ValidationError
if errors.As(err, &ve) {
    for _, fe := range ve.Errors {
        fmt.Printf("  %s: %s\n", fe.Key, fe.Msg)
    }
}
```

Validation rules are automatically appended to flag help text:

```
--port int   server port (min: 1, max: 65535) (default 8080)
```

## Pretty Print

Use `NewLoader` for advanced features like printing the resolved config table:

```go
loader := configs.NewLoader(
    configs.WithConfigFile("config.yaml"),
    configs.WithEnvPrefix("APP"),
)
if err := loader.Load(cfg); err != nil {
    log.Fatal(err)
}
loader.Print(os.Stdout)
```

```
┌───────────────────┬──────────────┬─────────┬──────────────────────────────────┐
│ KEY               │ VALUE        │ SOURCE  │ RULES                            │
├───────────────────┼──────────────┼─────────┼──────────────────────────────────┤
│ port              │ 3000         │ flag    │ min: 1, max: 65535               │
│ host              │ 0.0.0.0      │ file    │                                  │
│ log_level         │ debug        │ env     │ one of: [debug info warn error]  │
│ timeout           │ 30s          │ default │                                  │
│ database.host     │ db.prod      │ file    │                                  │
│ database.port     │ 5432         │ default │                                  │
│ database.name     │ myapp        │ file    │                                  │
│ database.password │ ********     │ env     │                                  │
└───────────────────┴──────────────┴─────────┴──────────────────────────────────┘
```

- **SOURCE** column shows where each value came from: `flag`, `env`, `file`, `default`, or `not set`
- Fields tagged `sensitive:"true"` are masked as `********`
- The RULES column is omitted when no field has validation rules

## Config File Watching

Watch the config file for changes and automatically reload:

```go
loader := configs.NewLoader(configs.WithConfigFile("config.yaml"))
if err := loader.Load(cfg); err != nil {
    log.Fatal(err)
}

loader.Watch(cfg, func(err error) {
    if err != nil {
        log.Println("config reload failed:", err)
        return // cfg retains its previous valid values
    }
    log.Println("config reloaded successfully")
})
```

- On file change, the new config is unmarshaled into a temporary copy and validated
- Only if both succeed is the original struct updated
- On error, the struct retains its previous values
- Env vars and flags still apply their precedence on reload

## Custom Types

Any type implementing `encoding.TextUnmarshaler` or `configs.Decoder` is treated as a leaf value parsed from a string. It works with all sources (defaults, YAML, env, flags).

### encoding.TextUnmarshaler

```go
type LogLevel struct {
    Level slog.Level
}

func (l *LogLevel) UnmarshalText(text []byte) error {
    return l.Level.UnmarshalText(text)
}

type Config struct {
    LogLevel LogLevel `yaml:"log_level" default:"INFO"`
}
```

### configs.Decoder

```go
type Endpoint struct {
    Scheme string
    Host   string
}

func (e *Endpoint) DecodeConfig(value string) error {
    scheme, host, ok := strings.Cut(value, "://")
    if !ok {
        return fmt.Errorf("invalid endpoint format: %s", value)
    }
    e.Scheme = scheme
    e.Host = host
    return nil
}

type Config struct {
    API Endpoint `yaml:"api" default:"https://api.example.com"`
}
```

Both interfaces:
- Are registered as string flags for CLI override
- Are not recursed into during struct field discovery
- Work with YAML scalar values, env vars, and flag strings

## Nested Structs and Embedding

Named nested structs add their yaml name as a prefix:

```go
type Config struct {
    Server struct {
        HTTP struct {
            Port int `yaml:"port" default:"8080"`
        } `yaml:"http"`
    } `yaml:"server"`
}
// YAML: server.http.port
// Env:  SERVER_HTTP_PORT
// Flag: --server-http-port
```

Anonymous (embedded) structs without an explicit yaml name are inlined — their fields are promoted to the parent level:

```go
type BaseConfig struct {
    Host string `yaml:"host" default:"localhost"`
    Port int    `yaml:"port" default:"8080"`
}

type Config struct {
    BaseConfig
    Name string `yaml:"name" default:"app"`
}
// host, port, and name are all at the top level
```

## Testing

Use `WithArgs` and `WithoutFlags` to make tests deterministic:

```go
func TestConfig(t *testing.T) {
    t.Setenv("APP_PORT", "3000")

    cfg := &Config{}
    err := configs.Load(cfg,
        configs.WithConfigFile("testdata/config.yaml"),
        configs.WithEnvPrefix("APP"),
        configs.WithArgs([]string{"--host", "127.0.0.1"}),
    )
    if err != nil {
        t.Fatal(err)
    }
    // assert cfg values...
}
```
