package configs

import (
	"context"
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
	v            *viper.Viper
	fs           *pflag.FlagSet
	opts         *options
	fields       []fieldInfo
	sourceVipers []*viper.Viper
	allSources   []configSource // opts.sources + resolved --config flag sources
	cfg          any
}

// NewLoader creates a Loader with the given options.
func NewLoader(opts ...Option) *Loader {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	return &Loader{opts: o}
}

// Load parses configuration into cfg from defaults, config sources, env vars, and flags.
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

	// Parse flags first so --config values are known before sources are merged.
	var configFlagValues []string
	if !l.opts.flagsDisabled {
		l.fs = pflag.NewFlagSet("config", pflag.ContinueOnError)

		for _, f := range l.fields {
			registerFlag(l.fs, f)
		}
		for _, fn := range l.opts.extraFlags {
			fn(l.fs)
		}
		if l.opts.configFlag != "" {
			l.fs.StringArray(l.opts.configFlag, nil,
				"config file path or URL (may be repeated; later values take precedence)")
		}

		args := l.opts.args
		if args == nil {
			args = os.Args[1:]
		}
		if err := l.fs.Parse(args); err != nil {
			return fmt.Errorf("configs: parsing flags: %w", err)
		}

		if l.opts.configFlag != "" {
			if vals, err := l.fs.GetStringArray(l.opts.configFlag); err == nil {
				configFlagValues = vals
			}
		}
	}

	// Merge explicit sources (opts.sources), then resolved --config flag sources.
	// Flag sources are appended last so they have higher precedence.
	cap := len(l.opts.sources) + len(configFlagValues)
	l.allSources = make([]configSource, 0, cap)
	l.sourceVipers = make([]*viper.Viper, 0, cap)

	for _, src := range l.opts.sources {
		sv, err := mergeSourceInto(l.v, src)
		if err != nil {
			return err
		}
		l.allSources = append(l.allSources, src)
		l.sourceVipers = append(l.sourceVipers, sv)
	}

	for _, val := range configFlagValues {
		src, err := l.resolveConfigValue(val)
		if err != nil {
			return fmt.Errorf("configs: resolving --%s %q: %w", l.opts.configFlag, val, err)
		}
		sv, err := mergeSourceInto(l.v, src)
		if err != nil {
			return err
		}
		l.allSources = append(l.allSources, src)
		l.sourceVipers = append(l.sourceVipers, sv)
	}

	for _, f := range l.fields {
		envKey := f.envKey
		if l.opts.envPrefix != "" {
			envKey = l.opts.envPrefix + "_" + f.envKey
		}
		_ = l.v.BindEnv(f.viperKey, envKey)
	}

	if l.fs != nil {
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

// Watch watches all registered config sources for changes and reloads cfg on each change.
// onChange receives nil on success or an error if reload/validation failed.
// On error, cfg retains its previous values.
func (l *Loader) Watch(cfg any, onChange func(error)) error {
	if len(l.allSources) == 0 {
		return fmt.Errorf("configs: Watch requires at least one config source (use WithConfigFile, WithRemote, or --%s)", l.opts.configFlag)
	}

	reload := func() {
		onChange(l.reload(cfg))
	}

	ctx := context.Background()

	for _, src := range l.allSources {
		switch src.kind {
		case sourceKindFile:
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				return fmt.Errorf("configs: creating file watcher: %w", err)
			}
			if err := watcher.Add(src.filePath); err != nil {
				_ = watcher.Close()
				return fmt.Errorf("configs: watching file %s: %w", src.filePath, err)
			}
			go func() {
				defer watcher.Close()
				for {
					select {
					case _, ok := <-watcher.Events:
						if !ok {
							return
						}
						reload()
					case err, ok := <-watcher.Errors:
						if !ok {
							return
						}
						onChange(err)
					}
				}
			}()
		case sourceKindRemote:
			src.provider.Watch(ctx, func(err error) {
				if err != nil {
					onChange(err)
					return
				}
				reload()
			})
		}
	}

	return nil
}

// reload re-fetches all sources in order, validates, and atomically swaps cfg on success.
func (l *Loader) reload(cfg any) error {
	newV := viper.New()
	newSourceVipers := make([]*viper.Viper, len(l.allSources))

	for _, f := range l.fields {
		if f.hasDefault {
			newV.SetDefault(f.viperKey, f.defaultVal)
		}
	}

	for i, src := range l.allSources {
		sv, err := mergeSourceInto(newV, src)
		if err != nil {
			return err
		}
		newSourceVipers[i] = sv
	}

	for _, f := range l.fields {
		envKey := f.envKey
		if l.opts.envPrefix != "" {
			envKey = l.opts.envPrefix + "_" + f.envKey
		}
		_ = newV.BindEnv(f.viperKey, envKey)
	}

	if l.fs != nil {
		for _, f := range l.fields {
			if flag := l.fs.Lookup(f.flagKey); flag != nil {
				_ = newV.BindPFlag(f.viperKey, flag)
			}
		}
	}

	var missing []string
	for _, f := range l.fields {
		if f.required && newV.Get(f.viperKey) == nil {
			missing = append(missing, f.viperKey)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("configs: required values not set: %s", strings.Join(missing, ", "))
	}

	rv := reflect.ValueOf(cfg)
	tmp := reflect.New(rv.Elem().Type())
	if err := newV.Unmarshal(tmp.Interface(), l.decoderOpt()); err != nil {
		return fmt.Errorf("configs: reloading: %w", err)
	}

	if err := validateStruct(tmp.Elem(), l.fields); err != nil {
		return err
	}

	l.v = newV
	l.sourceVipers = newSourceVipers
	rv.Elem().Set(tmp.Elem())
	l.cfg = cfg
	return nil
}

// Reload re-fetches all config sources and updates the struct last passed to Load.
// It applies the same safe-swap semantics as Watch: the struct is only updated
// if unmarshaling and validation both succeed. On error, the struct retains its previous values.
func (l *Loader) Reload() error {
	if l.cfg == nil {
		return fmt.Errorf("configs: Reload called before Load")
	}
	return l.reload(l.cfg)
}

// resolveConfigValue turns a --config flag value into a configSource.
// Values containing "://" are treated as URLs and routed to a registered scheme resolver.
// All other values are treated as file paths.
func (l *Loader) resolveConfigValue(val string) (configSource, error) {
	if idx := strings.Index(val, "://"); idx > 0 {
		scheme := val[:idx]
		resolve, ok := l.opts.urlSchemes[scheme]
		if !ok {
			return configSource{}, fmt.Errorf("no resolver registered for scheme %q (use WithURLScheme)", scheme)
		}
		provider, err := resolve(val)
		if err != nil {
			return configSource{}, err
		}
		return configSource{kind: sourceKindRemote, provider: provider}, nil
	}
	return configSource{kind: sourceKindFile, filePath: val}, nil
}

// mergeSourceInto reads src and merges it into v. It also returns a tracking viper
// populated with the same data (used by resolveSource to identify which source set a key).
func mergeSourceInto(v *viper.Viper, src configSource) (*viper.Viper, error) {
	sv := viper.New()
	switch src.kind {
	case sourceKindFile:
		if src.optional {
			if _, err := os.Stat(src.filePath); os.IsNotExist(err) {
				return sv, nil
			}
		}
		sv.SetConfigFile(src.filePath)
		if err := sv.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("configs: reading config file %s: %w", src.filePath, err)
		}
		if err := v.MergeConfigMap(sv.AllSettings()); err != nil {
			return nil, fmt.Errorf("configs: merging config file %s: %w", src.filePath, err)
		}
	case sourceKindRemote:
		data, err := src.provider.Fetch(context.Background())
		if err != nil {
			return nil, fmt.Errorf("configs: fetching remote config: %w", err)
		}
		if err := sv.MergeConfigMap(data); err != nil {
			return nil, fmt.Errorf("configs: setting remote config for tracking: %w", err)
		}
		if err := v.MergeConfigMap(data); err != nil {
			return nil, fmt.Errorf("configs: merging remote config: %w", err)
		}
	}
	return sv, nil
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
	for i := len(l.sourceVipers) - 1; i >= 0; i-- {
		if l.sourceVipers[i].IsSet(f.viperKey) {
			switch l.allSources[i].kind {
			case sourceKindFile:
				return "file"
			case sourceKindRemote:
				return "remote"
			}
		}
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
