package configs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Validation ---

func TestValidateMin(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"0" validate:"min=1"`
	}
	c := &cfg{}
	err := Load(c, WithoutFlags())
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *ValidationError
	if !errAs(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if ve.Errors[0].Key != "port" {
		t.Errorf("key = %q, want %q", ve.Errors[0].Key, "port")
	}
}

func TestValidateMax(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"70000" validate:"max=65535"`
	}
	c := &cfg{}
	err := Load(c, WithoutFlags())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "must be at most 65535") {
		t.Errorf("error = %q, want it to mention max", err.Error())
	}
}

func TestValidateMinMaxPass(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080" validate:"min=1,max=65535"`
	}
	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080", c.Port)
	}
}

func TestValidateOneof(t *testing.T) {
	type cfg struct {
		Level string `yaml:"level" default:"verbose" validate:"oneof=debug info warn error"`
	}
	c := &cfg{}
	err := Load(c, WithoutFlags())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("error = %q, want oneof message", err.Error())
	}
}

func TestValidateOneofPass(t *testing.T) {
	type cfg struct {
		Level string `yaml:"level" default:"info" validate:"oneof=debug info warn error"`
	}
	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Level != "info" {
		t.Errorf("Level = %q, want %q", c.Level, "info")
	}
}

func TestValidateNotempty(t *testing.T) {
	type cfg struct {
		Name string `yaml:"name" default:"" validate:"notempty"`
	}
	c := &cfg{}
	err := Load(c, WithoutFlags())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("error = %q, want notempty message", err.Error())
	}
}

func TestValidateMultipleFieldErrors(t *testing.T) {
	type cfg struct {
		Port  int    `yaml:"port" default:"0" validate:"min=1"`
		Level string `yaml:"level" default:"bad" validate:"oneof=debug info"`
	}
	c := &cfg{}
	err := Load(c, WithoutFlags())
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *ValidationError
	if !errAs(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if len(ve.Errors) != 2 {
		t.Errorf("got %d errors, want 2", len(ve.Errors))
	}
}

func TestValidateStringMinLength(t *testing.T) {
	type cfg struct {
		Token string `yaml:"token" default:"ab" validate:"min=3"`
	}
	c := &cfg{}
	err := Load(c, WithoutFlags())
	if err == nil {
		t.Fatal("expected validation error for short string")
	}
	if !strings.Contains(err.Error(), "length must be at least 3") {
		t.Errorf("error = %q, want length message", err.Error())
	}
}

// --- Describe Rules ---

func TestDescribeRules(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"min=1,max=100", "min: 1, max: 100"},
		{"oneof=a b c", "one of: [a b c]"},
		{"notempty", "non-empty"},
		{"min=0,notempty", "min: 0, non-empty"},
		{"", ""},
	}
	for _, tt := range tests {
		got := describeRules(parseRules(tt.tag))
		if got != tt.want {
			t.Errorf("describeRules(%q) = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

// --- Print ---

func TestLoaderPrint(t *testing.T) {
	type cfg struct {
		Port  int    `yaml:"port" default:"8080" validate:"min=1,max=65535"`
		Host  string `yaml:"host" default:"localhost"`
		Level string `yaml:"level" default:"info" validate:"oneof=debug info warn error"`
	}
	c := &cfg{}
	loader := NewLoader(WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	loader.Print(&buf)
	out := buf.String()

	for _, want := range []string{"KEY", "VALUE", "SOURCE", "RULES", "port", "8080", "default", "min: 1", "localhost", "one of:"} {
		if !strings.Contains(out, want) {
			t.Errorf("Print output missing %q:\n%s", want, out)
		}
	}
	// Should have box-drawing characters
	if !strings.Contains(out, "┌") || !strings.Contains(out, "┘") {
		t.Errorf("Print output missing table borders:\n%s", out)
	}
}

func TestLoaderPrintNoRulesColumn(t *testing.T) {
	type cfg struct {
		Port int    `yaml:"port" default:"8080"`
		Host string `yaml:"host" default:"localhost"`
	}
	c := &cfg{}
	loader := NewLoader(WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	loader.Print(&buf)
	out := buf.String()

	if strings.Contains(out, "RULES") {
		t.Errorf("Print should omit RULES column when no validation rules exist:\n%s", out)
	}
}

func TestLoaderPrintSensitive(t *testing.T) {
	type cfg struct {
		Password string `yaml:"password" default:"s3cret" sensitive:"true"`
	}
	c := &cfg{}
	loader := NewLoader(WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	loader.Print(&buf)
	out := buf.String()

	if strings.Contains(out, "s3cret") {
		t.Errorf("Print should mask sensitive values:\n%s", out)
	}
	if !strings.Contains(out, "********") {
		t.Errorf("Print should show ******** for sensitive values:\n%s", out)
	}
}

// --- Source Resolution ---

func TestLoaderSourceDefault(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	loader := NewLoader(WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	loader.Print(&buf)
	if !strings.Contains(buf.String(), "default") {
		t.Errorf("expected source 'default' in output:\n%s", buf.String())
	}
}

func TestLoaderSourceEnv(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	t.Setenv("APP_PORT", "3000")

	c := &cfg{}
	loader := NewLoader(WithEnvPrefix("APP"), WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	loader.Print(&buf)
	if !strings.Contains(buf.String(), "env") {
		t.Errorf("expected source 'env' in output:\n%s", buf.String())
	}
}

func TestLoaderSourceFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgFile, []byte("port: 9090\n"), 0644)

	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	loader := NewLoader(WithConfigFile(cfgFile), WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	loader.Print(&buf)
	if !strings.Contains(buf.String(), "file") {
		t.Errorf("expected source 'file' in output:\n%s", buf.String())
	}
}

func TestLoaderSourceFlag(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	loader := NewLoader(WithArgs([]string{"--port", "4000"}))
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	loader.Print(&buf)
	if !strings.Contains(buf.String(), "flag") {
		t.Errorf("expected source 'flag' in output:\n%s", buf.String())
	}
}

// --- Watch ---

func TestLoaderWatch(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgFile, []byte("port: 8080\n"), 0644)

	type cfg struct {
		Port int `yaml:"port" default:"3000"`
	}
	c := &cfg{}
	loader := NewLoader(WithConfigFile(cfgFile), WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}
	if c.Port != 8080 {
		t.Fatalf("Port = %d, want 8080", c.Port)
	}

	done := make(chan error, 1)
	if err := loader.Watch(c, func(err error) {
		done <- err
	}); err != nil {
		t.Fatal(err)
	}

	// Let watcher start
	time.Sleep(200 * time.Millisecond)

	os.WriteFile(cfgFile, []byte("port: 9090\n"), 0644)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Watch callback error: %v", err)
		}
		if c.Port != 9090 {
			t.Errorf("Port = %d, want 9090 after reload", c.Port)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Watch callback not called within 5 seconds")
	}
}

func TestLoaderWatchValidationError(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgFile, []byte("port: 8080\n"), 0644)

	type cfg struct {
		Port int `yaml:"port" default:"3000" validate:"min=1,max=65535"`
	}
	c := &cfg{}
	loader := NewLoader(WithConfigFile(cfgFile), WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	if err := loader.Watch(c, func(err error) {
		done <- err
	}); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Write invalid value
	os.WriteFile(cfgFile, []byte("port: 70000\n"), 0644)

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected validation error from Watch")
		}
		// On validation error, cfg should retain old value
		if c.Port != 8080 {
			t.Errorf("Port = %d, want 8080 (unchanged on validation error)", c.Port)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Watch callback not called within 5 seconds")
	}
}

func TestLoaderWatchNoConfigFile(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	loader := NewLoader(WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}
	if err := loader.Watch(c, func(error) {}); err == nil {
		t.Fatal("expected error when Watch called without config file")
	}
}

// --- Custom Types: TextUnmarshaler ---

type LogLevel struct {
	Name string
}

func (l *LogLevel) UnmarshalText(text []byte) error {
	s := string(text)
	switch s {
	case "debug", "info", "warn", "error":
		l.Name = s
		return nil
	default:
		return fmt.Errorf("invalid log level: %s", s)
	}
}

func TestTextUnmarshaler(t *testing.T) {
	type cfg struct {
		Level LogLevel `yaml:"level" default:"info"`
	}
	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Level.Name != "info" {
		t.Errorf("Level.Name = %q, want %q", c.Level.Name, "info")
	}
}

func TestTextUnmarshalerFromEnv(t *testing.T) {
	type cfg struct {
		Level LogLevel `yaml:"level" default:"info"`
	}
	t.Setenv("LEVEL", "debug")
	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Level.Name != "debug" {
		t.Errorf("Level.Name = %q, want %q", c.Level.Name, "debug")
	}
}

func TestTextUnmarshalerFromFlag(t *testing.T) {
	type cfg struct {
		Level LogLevel `yaml:"level" default:"info"`
	}
	c := &cfg{}
	if err := Load(c, WithArgs([]string{"--level", "warn"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Level.Name != "warn" {
		t.Errorf("Level.Name = %q, want %q", c.Level.Name, "warn")
	}
}

func TestTextUnmarshalerFromYAML(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgFile, []byte("level: error\n"), 0644)

	type cfg struct {
		Level LogLevel `yaml:"level" default:"info"`
	}
	c := &cfg{}
	if err := Load(c, WithConfigFile(cfgFile), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Level.Name != "error" {
		t.Errorf("Level.Name = %q, want %q", c.Level.Name, "error")
	}
}

// --- Custom Types: Decoder ---

type Endpoint struct {
	Scheme string
	Host   string
}

func (e *Endpoint) DecodeConfig(value string) error {
	scheme, host, ok := strings.Cut(value, "://")
	if !ok {
		return fmt.Errorf("invalid endpoint: %s", value)
	}
	e.Scheme = scheme
	e.Host = host
	return nil
}

func TestDecoder(t *testing.T) {
	type cfg struct {
		API Endpoint `yaml:"api" default:"https://api.example.com"`
	}
	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.API.Scheme != "https" {
		t.Errorf("API.Scheme = %q, want %q", c.API.Scheme, "https")
	}
	if c.API.Host != "api.example.com" {
		t.Errorf("API.Host = %q, want %q", c.API.Host, "api.example.com")
	}
}

func TestDecoderFromEnv(t *testing.T) {
	type cfg struct {
		API Endpoint `yaml:"api" default:"https://default.com"`
	}
	t.Setenv("API", "grpc://backend:9090")
	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.API.Scheme != "grpc" {
		t.Errorf("API.Scheme = %q, want %q", c.API.Scheme, "grpc")
	}
	if c.API.Host != "backend:9090" {
		t.Errorf("API.Host = %q, want %q", c.API.Host, "backend:9090")
	}
}

func TestDecoderFromFlag(t *testing.T) {
	type cfg struct {
		API Endpoint `yaml:"api" default:"https://default.com"`
	}
	c := &cfg{}
	if err := Load(c, WithArgs([]string{"--api", "http://localhost:8080"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.API.Scheme != "http" {
		t.Errorf("API.Scheme = %q, want %q", c.API.Scheme, "http")
	}
	if c.API.Host != "localhost:8080" {
		t.Errorf("API.Host = %q, want %q", c.API.Host, "localhost:8080")
	}
}

// --- Validation rules in flag help ---

func TestValidateRulesInFlagHelp(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080" description:"server port" validate:"min=1,max=65535"`
	}
	loader := NewLoader(WithArgs([]string{}))
	c := &cfg{}
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}
	flag := loader.fs.Lookup("port")
	if flag == nil {
		t.Fatal("flag 'port' not found")
	}
	if !strings.Contains(flag.Usage, "min: 1") || !strings.Contains(flag.Usage, "max: 65535") {
		t.Errorf("flag usage = %q, want it to contain validation rules", flag.Usage)
	}
}

// --- helpers ---

func errAs(err error, target any) bool {
	// Type-assert to *ValidationError
	if ve, ok := err.(*ValidationError); ok {
		if t, ok2 := target.(**ValidationError); ok2 {
			*t = ve
			return true
		}
	}
	return false
}
