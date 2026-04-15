package configs

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type rule struct {
	name  string
	param string
}

type fieldError struct {
	Key string
	Msg string
}

// ValidationError contains per-field validation failures returned by Load.
type ValidationError struct {
	Errors []fieldError
}

func (e *ValidationError) Error() string {
	var buf strings.Builder
	buf.WriteString("configs: validation failed:")
	for _, fe := range e.Errors {
		buf.WriteString("\n  ")
		buf.WriteString(fe.Key)
		buf.WriteString(": ")
		buf.WriteString(fe.Msg)
	}
	return buf.String()
}

func parseRules(tag string) []rule {
	if tag == "" {
		return nil
	}
	var rules []rule
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, param, _ := strings.Cut(part, "=")
		rules = append(rules, rule{name: name, param: param})
	}
	return rules
}

func validateStruct(rv reflect.Value, fields []fieldInfo) error {
	var errs []fieldError
	for _, f := range fields {
		if len(f.rules) == 0 {
			continue
		}
		val := rv.FieldByIndex(f.index)
		for _, r := range f.rules {
			if msg := applyRule(val, r); msg != "" {
				errs = append(errs, fieldError{Key: f.viperKey, Msg: msg})
				break
			}
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

func applyRule(val reflect.Value, r rule) string {
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}
	switch r.name {
	case "min":
		return checkMin(val, r.param)
	case "max":
		return checkMax(val, r.param)
	case "oneof":
		return checkOneOf(val, r.param)
	case "notempty":
		return checkNotEmpty(val)
	}
	return ""
}

func checkMin(val reflect.Value, param string) string {
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		min, _ := strconv.ParseInt(param, 10, 64)
		if val.Int() < min {
			return fmt.Sprintf("must be at least %s, got %d", param, val.Int())
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		min, _ := strconv.ParseUint(param, 10, 64)
		if val.Uint() < min {
			return fmt.Sprintf("must be at least %s, got %d", param, val.Uint())
		}
	case reflect.Float32, reflect.Float64:
		min, _ := strconv.ParseFloat(param, 64)
		if val.Float() < min {
			return fmt.Sprintf("must be at least %s, got %v", param, val.Float())
		}
	case reflect.String:
		min, _ := strconv.Atoi(param)
		if len(val.String()) < min {
			return fmt.Sprintf("length must be at least %s, got %d", param, len(val.String()))
		}
	case reflect.Slice, reflect.Array:
		min, _ := strconv.Atoi(param)
		if val.Len() < min {
			return fmt.Sprintf("length must be at least %s, got %d", param, val.Len())
		}
	}
	return ""
}

func checkMax(val reflect.Value, param string) string {
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		max, _ := strconv.ParseInt(param, 10, 64)
		if val.Int() > max {
			return fmt.Sprintf("must be at most %s, got %d", param, val.Int())
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		max, _ := strconv.ParseUint(param, 10, 64)
		if val.Uint() > max {
			return fmt.Sprintf("must be at most %s, got %d", param, val.Uint())
		}
	case reflect.Float32, reflect.Float64:
		max, _ := strconv.ParseFloat(param, 64)
		if val.Float() > max {
			return fmt.Sprintf("must be at most %s, got %v", param, val.Float())
		}
	case reflect.String:
		max, _ := strconv.Atoi(param)
		if len(val.String()) > max {
			return fmt.Sprintf("length must be at most %s, got %d", param, len(val.String()))
		}
	case reflect.Slice, reflect.Array:
		max, _ := strconv.Atoi(param)
		if val.Len() > max {
			return fmt.Sprintf("length must be at most %s, got %d", param, val.Len())
		}
	}
	return ""
}

func checkOneOf(val reflect.Value, param string) string {
	allowed := strings.Fields(param)
	s := fmt.Sprintf("%v", val.Interface())
	for _, a := range allowed {
		if s == a {
			return ""
		}
	}
	return fmt.Sprintf("must be one of [%s], got %q", strings.Join(allowed, " "), s)
}

func checkNotEmpty(val reflect.Value) string {
	switch val.Kind() {
	case reflect.String:
		if val.String() == "" {
			return "must not be empty"
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		if val.Len() == 0 {
			return "must not be empty"
		}
	}
	return ""
}

func describeRules(rules []rule) string {
	if len(rules) == 0 {
		return ""
	}
	var parts []string
	for _, r := range rules {
		switch r.name {
		case "min":
			parts = append(parts, "min: "+r.param)
		case "max":
			parts = append(parts, "max: "+r.param)
		case "oneof":
			parts = append(parts, "one of: ["+strings.Join(strings.Fields(r.param), " ")+"]")
		case "notempty":
			parts = append(parts, "non-empty")
		}
	}
	return strings.Join(parts, ", ")
}
