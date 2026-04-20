package configs

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

// WriteSample writes a documented YAML template derived from cfg's struct tags to w.
// Each field is shown with its default value (or a zero placeholder if no default is set),
// with description, validation rules, required, and sensitive annotations as comments.
// Useful for bootstrapping a config file or generating reference documentation.
//
//	configs.WriteSample(os.Stdout, &Config{})
func WriteSample(w io.Writer, cfg any) error {
	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("configs: WriteSample requires a pointer to a struct, got %T", cfg)
	}
	fields := discoverFields(rv.Elem().Type(), "", nil)
	byKey := make(map[string]fieldInfo, len(fields))
	for _, f := range fields {
		byKey[f.viperKey] = f
	}
	writeSampleStruct(w, rv.Elem().Type(), 0, "", byKey)
	return nil
}

// WriteSample writes a documented YAML template for the struct last passed to Load.
func (l *Loader) WriteSample(w io.Writer) error {
	if l.cfg == nil {
		return fmt.Errorf("configs: WriteSample called before Load")
	}
	return WriteSample(w, l.cfg)
}

func writeSampleStruct(w io.Writer, t reflect.Type, indent int, prefix string, byKey map[string]fieldInfo) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	ind := strings.Repeat("  ", indent)

	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}

		name := yamlFieldName(sf)
		if name == "-" {
			continue
		}

		ft := sf.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		isAnon := sf.Anonymous && !hasExplicitYAMLName(sf)

		if ft.Kind() == reflect.Struct && !isCustomType(ft) {
			if isAnon {
				writeSampleStruct(w, ft, indent, prefix, byKey)
			} else {
				nestedPrefix := name
				if prefix != "" {
					nestedPrefix = prefix + "." + name
				}
				if indent == 0 {
					fmt.Fprintln(w)
				}
				fmt.Fprintf(w, "%s%s:\n", ind, name)
				writeSampleStruct(w, ft, indent+1, nestedPrefix, byKey)
			}
			continue
		}

		viperKey := name
		if prefix != "" {
			viperKey = prefix + "." + name
		}

		f, ok := byKey[viperKey]
		if !ok {
			continue
		}

		if indent == 0 {
			fmt.Fprintln(w)
		}
		if f.description != "" {
			fmt.Fprintf(w, "%s# %s\n", ind, f.description)
		}
		if rules := describeRules(f.rules); rules != "" {
			fmt.Fprintf(w, "%s# %s\n", ind, rules)
		}
		if f.required {
			fmt.Fprintf(w, "%s# required\n", ind)
		}
		if f.sensitive {
			fmt.Fprintf(w, "%s# sensitive\n", ind)
		}
		fmt.Fprintf(w, "%s%s: %s\n", ind, name, sampleValue(f))
	}
}

func sampleValue(f fieldInfo) string {
	t := f.fieldType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if !f.hasDefault {
		switch t.Kind() {
		case reflect.Bool:
			return "false"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return "0"
		case reflect.Float32, reflect.Float64:
			return "0.0"
		default:
			return `""`
		}
	}

	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return f.defaultVal
	case reflect.String:
		return `"` + f.defaultVal + `"`
	default:
		// Durations, custom types: the default is already a parseable string value.
		if f.defaultVal == "" {
			return `""`
		}
		return f.defaultVal
	}
}
