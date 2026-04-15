package configs

import (
	"encoding"
	"reflect"
	"strings"
	"unicode"
)

type fieldInfo struct {
	viperKey    string
	envKey      string
	flagKey     string
	flagShort   string
	defaultVal  string
	hasDefault  bool
	required    bool
	sensitive   bool
	description string
	rules       []rule
	fieldType   reflect.Type
	index       []int
}

var (
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	decoderType         = reflect.TypeOf((*Decoder)(nil)).Elem()
)

func isCustomType(t reflect.Type) bool {
	return t.Implements(textUnmarshalerType) ||
		reflect.PointerTo(t).Implements(textUnmarshalerType) ||
		t.Implements(decoderType) ||
		reflect.PointerTo(t).Implements(decoderType)
}

func discoverFields(t reflect.Type, prefix string, indexPrefix []int) []fieldInfo {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	var fields []fieldInfo
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}

		idx := append(append([]int{}, indexPrefix...), i)

		ft := sf.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		if ft.Kind() == reflect.Struct && !isCustomType(ft) {
			if sf.Anonymous && !hasExplicitYAMLName(sf) {
				fields = append(fields, discoverFields(ft, prefix, idx)...)
			} else {
				name := yamlFieldName(sf)
				if name == "-" {
					continue
				}
				key := name
				if prefix != "" {
					key = prefix + "." + name
				}
				fields = append(fields, discoverFields(ft, key, idx)...)
			}
			continue
		}

		name := yamlFieldName(sf)
		if name == "-" {
			continue
		}

		viperKey := name
		if prefix != "" {
			viperKey = prefix + "." + name
		}

		envLeaf := strings.ToUpper(name)
		if tag := sf.Tag.Get("env"); tag != "" {
			envLeaf = tag
		}
		envPrefix := strings.ToUpper(strings.ReplaceAll(prefix, ".", "_"))
		envKey := envLeaf
		if envPrefix != "" {
			envKey = envPrefix + "_" + envLeaf
		}

		flagLeaf := name
		var flagShort string
		if tag := sf.Tag.Get("flag"); tag != "" {
			if len(tag) == 1 {
				flagShort = tag
			} else {
				flagLeaf = tag
			}
		}
		flagPrefix := strings.ReplaceAll(prefix, ".", "-")
		flagKey := flagLeaf
		if flagPrefix != "" {
			flagKey = flagPrefix + "-" + flagLeaf
		}

		defaultVal, hasDefault := sf.Tag.Lookup("default")

		fields = append(fields, fieldInfo{
			viperKey:    viperKey,
			envKey:      envKey,
			flagKey:     flagKey,
			flagShort:   flagShort,
			defaultVal:  defaultVal,
			hasDefault:  hasDefault,
			required:    sf.Tag.Get("required") == "true",
			sensitive:   sf.Tag.Get("sensitive") == "true",
			description: sf.Tag.Get("description"),
			rules:       parseRules(sf.Tag.Get("validate")),
			fieldType:   sf.Type,
			index:       idx,
		})
	}
	return fields
}

func hasExplicitYAMLName(sf reflect.StructField) bool {
	tag := sf.Tag.Get("yaml")
	if tag == "" {
		return false
	}
	name := tag
	if idx := strings.Index(tag, ","); idx != -1 {
		name = tag[:idx]
	}
	return name != ""
}

func yamlFieldName(sf reflect.StructField) string {
	tag := sf.Tag.Get("yaml")
	if tag == "-" {
		return "-"
	}
	if tag != "" {
		name := tag
		if idx := strings.Index(tag, ","); idx != -1 {
			name = tag[:idx]
		}
		if name != "" {
			return name
		}
	}
	return toSnakeCase(sf.Name)
}

func toSnakeCase(s string) string {
	var buf strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			buf.WriteRune('_')
		}
		buf.WriteRune(unicode.ToLower(r))
	}
	return buf.String()
}
