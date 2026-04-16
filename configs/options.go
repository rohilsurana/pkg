package configs

import "strings"

type sourceKind int

const (
	sourceKindFile sourceKind = iota
	sourceKindRemote
)

type configSource struct {
	kind     sourceKind
	filePath string         // sourceKindFile
	provider RemoteProvider // sourceKindRemote
}

type options struct {
	sources       []configSource
	envPrefix     string
	args          []string
	flagsDisabled bool
}

// Option configures how Load behaves.
type Option func(*options)

func defaultOptions() *options {
	return &options{}
}

// WithConfigFile adds a YAML config file as a config source at the current precedence position.
// Load returns an error if the file does not exist or cannot be parsed.
// Multiple calls are allowed; later calls have higher precedence over earlier ones.
func WithConfigFile(path string) Option {
	return func(o *options) {
		o.sources = append(o.sources, configSource{
			kind:     sourceKindFile,
			filePath: path,
		})
	}
}

// WithEnvPrefix sets a prefix for environment variable lookups.
// For example, WithEnvPrefix("APP") makes the field "port" look for APP_PORT.
func WithEnvPrefix(prefix string) Option {
	return func(o *options) {
		o.envPrefix = strings.ToUpper(prefix)
	}
}

// WithArgs overrides os.Args[1:] for flag parsing. Useful for testing.
func WithArgs(args []string) Option {
	return func(o *options) {
		o.args = args
	}
}

// WithoutFlags disables command-line flag parsing entirely.
func WithoutFlags() Option {
	return func(o *options) {
		o.flagsDisabled = true
	}
}
