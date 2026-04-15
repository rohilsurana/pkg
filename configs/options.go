package configs

import "strings"

type options struct {
	configFile    string
	envPrefix     string
	args          []string
	flagsDisabled bool
}

// Option configures how Load behaves.
type Option func(*options)

func defaultOptions() *options {
	return &options{}
}

// WithConfigFile sets the path to a YAML config file.
// Load returns an error if the file does not exist or cannot be parsed.
func WithConfigFile(path string) Option {
	return func(o *options) {
		o.configFile = path
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
