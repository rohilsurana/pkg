package configs

import (
	"strings"

	"github.com/spf13/pflag"
)

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
	configFlag    string
	urlSchemes    map[string]func(string) (RemoteProvider, error)
	extraFlags    []func(*pflag.FlagSet)
}

// Option configures how Load behaves.
type Option func(*options)

func defaultOptions() *options {
	return &options{
		urlSchemes: make(map[string]func(string) (RemoteProvider, error)),
	}
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

// WithConfigFlag enables a built-in flag that accepts config file paths or scheme://... URLs.
// Values are loaded after explicit WithConfigFile/WithRemote sources (higher precedence).
// The flag may be repeated; later values have higher precedence over earlier ones.
//
//	configs.WithConfigFlag("config")
//	// ./myapp --config base.yaml --config etcd://host/key --config local.yaml
func WithConfigFlag(name string) Option {
	return func(o *options) {
		o.configFlag = name
	}
}

// WithURLScheme registers a resolver for a URL scheme used in --config values.
// When --config receives a value matching scheme://..., the resolver is called to
// produce a RemoteProvider. The resolver runs after flags are parsed, so variables
// captured in a closure already hold their flag values.
//
//	var token string
//	configs.WithExtraFlags(func(fs *pflag.FlagSet) {
//	    fs.StringVar(&token, "config-token", os.Getenv("TOKEN"), "bearer token")
//	})
//	configs.WithURLScheme("https", func(rawURL string) (configs.RemoteProvider, error) {
//	    return NewHTTPProvider(rawURL, token), nil
//	})
func WithURLScheme(scheme string, resolve func(rawURL string) (RemoteProvider, error)) Option {
	return func(o *options) {
		o.urlSchemes[scheme] = resolve
	}
}

// WithExtraFlags registers additional flags on the loader's internal FlagSet.
// The callback runs during flag registration (before parsing), so flag variables
// populated here are available to URL scheme resolvers and load hooks.
func WithExtraFlags(fn func(*pflag.FlagSet)) Option {
	return func(o *options) {
		o.extraFlags = append(o.extraFlags, fn)
	}
}
