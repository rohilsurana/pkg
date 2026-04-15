package configs

// Load is a convenience function that parses configuration into cfg.
// For advanced features (Print, Watch), use NewLoader instead.
//
// cfg must be a pointer to a struct. Sources in order of increasing precedence:
//
//	defaults (struct tags) < config file (YAML) < env vars < flags
//
// Supported struct tags:
//
//	yaml:"name"           — YAML key name; also base for deriving env/flag names
//	default:"value"       — default value when no source provides one
//	required:"true"       — error if no source provides a value and no default is set
//	env:"NAME"            — override the auto-derived env var leaf name
//	flag:"name"           — override the auto-derived flag leaf name (single char = shorthand)
//	description:"text"    — help text shown for the flag
//	sensitive:"true"      — mask value in Print output
//	validate:"rules"      — validation rules: min, max, oneof, notempty
func Load(cfg any, opts ...Option) error {
	return NewLoader(opts...).Load(cfg)
}

// Decoder is implemented by types that can decode themselves from a config string.
// This is checked during unmarshaling alongside encoding.TextUnmarshaler.
type Decoder interface {
	DecodeConfig(value string) error
}
