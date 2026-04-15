package configs

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Loader holds the state for loading, printing, and watching configuration.
type Loader struct {
	v         *viper.Viper
	fs        *pflag.FlagSet
	opts      *options
	fields    []fieldInfo
	fileViper *viper.Viper
	cfg       any
}

// NewLoader creates a Loader with the given options.
func NewLoader(opts ...Option) *Loader {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	return &Loader{opts: o}
}

// Load parses configuration into cfg from defaults, config file, env vars, and flags.
// cfg must be a pointer to a struct.
func (l *Loader) Load(cfg any) error {
	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("configs: Load requires a pointer to a struct, got %T", cfg)
	}

	l.v = viper.New()
	l.cfg = cfg
	l.fields = discoverFields(rv.Elem().Type(), "", nil)

	for _, f := range l.fields {
		if f.hasDefault {
			l.v.SetDefault(f.viperKey, f.defaultVal)
		}
	}

	if l.opts.configFile != "" {
		l.v.SetConfigFile(l.opts.configFile)
		if err := l.v.ReadInConfig(); err != nil {
			return fmt.Errorf("configs: reading config file: %w", err)
		}
		l.fileViper = viper.New()
		l.fileViper.SetConfigFile(l.opts.configFile)
		_ = l.fileViper.ReadInConfig()
	}

	for _, f := range l.fields {
		envKey := f.envKey
		if l.opts.envPrefix != "" {
			envKey = l.opts.envPrefix + "_" + f.envKey
		}
		_ = l.v.BindEnv(f.viperKey, envKey)
	}

	if !l.opts.flagsDisabled {
		l.fs = pflag.NewFlagSet("config", pflag.ContinueOnError)
		for _, f := range l.fields {
			registerFlag(l.fs, f)
		}
		args := l.opts.args
		if args == nil {
			args = os.Args[1:]
		}
		if err := l.fs.Parse(args); err != nil {
			return fmt.Errorf("configs: parsing flags: %w", err)
		}
		for _, f := range l.fields {
			if flag := l.fs.Lookup(f.flagKey); flag != nil {
				_ = l.v.BindPFlag(f.viperKey, flag)
			}
		}
	}

	var missing []string
	for _, f := range l.fields {
		if f.required && l.v.Get(f.viperKey) == nil {
			missing = append(missing, f.viperKey)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("configs: required values not set: %s", strings.Join(missing, ", "))
	}

	if err := l.v.Unmarshal(cfg, l.decoderOpt()); err != nil {
		return fmt.Errorf("configs: unmarshaling: %w", err)
	}

	if err := validateStruct(rv.Elem(), l.fields); err != nil {
		return err
	}

	return nil
}

// Watch watches the config file for changes and reloads cfg on each change.
// onChange receives nil on success or an error if reload/validation failed.
// On error, cfg retains its previous values.
func (l *Loader) Watch(cfg any, onChange func(error)) error {
	if l.opts.configFile == "" {
		return fmt.Errorf("configs: Watch requires a config file (use WithConfigFile)")
	}

	l.v.OnConfigChange(func(_ fsnotify.Event) {
		l.fileViper = viper.New()
		l.fileViper.SetConfigFile(l.opts.configFile)
		_ = l.fileViper.ReadInConfig()

		var missing []string
		for _, f := range l.fields {
			if f.required && l.v.Get(f.viperKey) == nil {
				missing = append(missing, f.viperKey)
			}
		}
		if len(missing) > 0 {
			onChange(fmt.Errorf("configs: required values not set: %s", strings.Join(missing, ", ")))
			return
		}

		rv := reflect.ValueOf(cfg)
		tmp := reflect.New(rv.Elem().Type())
		if err := l.v.Unmarshal(tmp.Interface(), l.decoderOpt()); err != nil {
			onChange(fmt.Errorf("configs: reloading: %w", err))
			return
		}

		if err := validateStruct(tmp.Elem(), l.fields); err != nil {
			onChange(err)
			return
		}

		rv.Elem().Set(tmp.Elem())
		l.cfg = cfg
		onChange(nil)
	})
	l.v.WatchConfig()
	return nil
}

func (l *Loader) resolveSource(f fieldInfo) string {
	if l.fs != nil {
		if flag := l.fs.Lookup(f.flagKey); flag != nil && flag.Changed {
			return "flag"
		}
	}
	envKey := f.envKey
	if l.opts.envPrefix != "" {
		envKey = l.opts.envPrefix + "_" + f.envKey
	}
	if _, ok := os.LookupEnv(envKey); ok {
		return "env"
	}
	if l.fileViper != nil && l.fileViper.IsSet(f.viperKey) {
		return "file"
	}
	if f.hasDefault {
		return "default"
	}
	return "not set"
}

func (l *Loader) decoderOpt() viper.DecoderConfigOption {
	return func(dc *mapstructure.DecoderConfig) {
		existing := dc.DecodeHook
		dc.TagName = "yaml"
		dc.Squash = true
		dc.MatchName = func(mapKey, fieldName string) bool {
			normalize := func(s string) string {
				return strings.ToLower(strings.ReplaceAll(s, "_", ""))
			}
			return normalize(mapKey) == normalize(fieldName)
		}
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			existing,
			textUnmarshalerHook(),
			configDecoderHook(),
		)
	}
}

// textUnmarshalerHook converts string values to types implementing encoding.TextUnmarshaler.
func textUnmarshalerHook() mapstructure.DecodeHookFuncType {
	return func(from, to reflect.Type, data any) (any, error) {
		if from.Kind() != reflect.String {
			return data, nil
		}
		val := reflect.New(to)
		u, ok := val.Interface().(interface{ UnmarshalText([]byte) error })
		if !ok {
			return data, nil
		}
		if err := u.UnmarshalText([]byte(data.(string))); err != nil {
			return nil, err
		}
		return val.Elem().Interface(), nil
	}
}

// configDecoderHook converts string values to types implementing Decoder.
func configDecoderHook() mapstructure.DecodeHookFuncType {
	return func(from, to reflect.Type, data any) (any, error) {
		if from.Kind() != reflect.String {
			return data, nil
		}
		if !reflect.PointerTo(to).Implements(decoderType) {
			return data, nil
		}
		val := reflect.New(to)
		if err := val.Interface().(Decoder).DecodeConfig(data.(string)); err != nil {
			return nil, err
		}
		return val.Elem().Interface(), nil
	}
}

func registerFlag(fs *pflag.FlagSet, f fieldInfo) {
	t := f.fieldType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	desc := f.description
	if desc == "" {
		desc = f.viperKey
	}
	if rd := describeRules(f.rules); rd != "" {
		desc += " (" + rd + ")"
	}

	sh := f.flagShort

	switch t.Kind() {
	case reflect.String:
		fs.StringP(f.flagKey, sh, f.defaultVal, desc)
	case reflect.Int:
		def, _ := strconv.Atoi(f.defaultVal)
		fs.IntP(f.flagKey, sh, def, desc)
	case reflect.Int8, reflect.Int16, reflect.Int32:
		def, _ := strconv.ParseInt(f.defaultVal, 10, 32)
		fs.Int32P(f.flagKey, sh, int32(def), desc)
	case reflect.Int64:
		if t == reflect.TypeOf(time.Duration(0)) {
			def, _ := time.ParseDuration(f.defaultVal)
			fs.DurationP(f.flagKey, sh, def, desc)
		} else {
			def, _ := strconv.ParseInt(f.defaultVal, 10, 64)
			fs.Int64P(f.flagKey, sh, def, desc)
		}
	case reflect.Uint:
		def, _ := strconv.ParseUint(f.defaultVal, 10, 64)
		fs.UintP(f.flagKey, sh, uint(def), desc)
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		def, _ := strconv.ParseUint(f.defaultVal, 10, 32)
		fs.Uint32P(f.flagKey, sh, uint32(def), desc)
	case reflect.Uint64:
		def, _ := strconv.ParseUint(f.defaultVal, 10, 64)
		fs.Uint64P(f.flagKey, sh, def, desc)
	case reflect.Float32:
		def, _ := strconv.ParseFloat(f.defaultVal, 32)
		fs.Float32P(f.flagKey, sh, float32(def), desc)
	case reflect.Float64:
		def, _ := strconv.ParseFloat(f.defaultVal, 64)
		fs.Float64P(f.flagKey, sh, def, desc)
	case reflect.Bool:
		def, _ := strconv.ParseBool(f.defaultVal)
		fs.BoolP(f.flagKey, sh, def, desc)
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			var def []string
			if f.defaultVal != "" {
				def = strings.Split(f.defaultVal, ",")
			}
			fs.StringSliceP(f.flagKey, sh, def, desc)
		}
	default:
		if isCustomType(t) {
			fs.StringP(f.flagKey, sh, f.defaultVal, desc)
		}
	}
}
