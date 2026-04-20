package configs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

// fakeRemoteProvider is a test double for RemoteProvider.
type fakeRemoteProvider struct {
	mu       sync.Mutex
	data     map[string]any
	fetchErr error
	watchers []func(error)
}

func newFakeRemote(data map[string]any) *fakeRemoteProvider {
	return &fakeRemoteProvider{data: data}
}

func (f *fakeRemoteProvider) Fetch(_ context.Context) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	out := make(map[string]any, len(f.data))
	for k, v := range f.data {
		out[k] = v
	}
	return out, nil
}

func (f *fakeRemoteProvider) Watch(_ context.Context, onChange func(error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.watchers = append(f.watchers, onChange)
}

// update sets new data and notifies all watchers.
func (f *fakeRemoteProvider) update(data map[string]any) {
	f.mu.Lock()
	f.data = data
	watchers := make([]func(error), len(f.watchers))
	copy(watchers, f.watchers)
	f.mu.Unlock()
	for _, w := range watchers {
		w(nil)
	}
}

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

// --- WithOptionalConfigFile ---

func TestOptionalConfigFileMissing(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	// Non-existent file with WithOptionalConfigFile must not error
	if err := Load(c, WithOptionalConfigFile("/nonexistent/config.yaml"), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error for missing optional file: %v", err)
	}
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080 (default)", c.Port)
	}
}

func TestOptionalConfigFilePresent(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgFile, []byte("port: 9090\n"), 0644)

	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	if err := Load(c, WithOptionalConfigFile(cfgFile), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090", c.Port)
	}
}

func TestOptionalConfigFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	local := filepath.Join(dir, "local.yaml")
	os.WriteFile(base, []byte("port: 7070\nhost: base\n"), 0644)
	os.WriteFile(local, []byte("port: 9090\n"), 0644)

	type cfg struct {
		Port int    `yaml:"port" default:"8080"`
		Host string `yaml:"host" default:"localhost"`
	}
	c := &cfg{}
	err := Load(c,
		WithConfigFile(base),
		WithOptionalConfigFile(local), // local overrides base when present
		WithoutFlags(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090 (local overrides base)", c.Port)
	}
	if c.Host != "base" {
		t.Errorf("Host = %q, want %q (base value preserved when local doesn't set it)", c.Host, "base")
	}
}

func TestOptionalConfigFileBadSyntax(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "bad.yaml")
	os.WriteFile(cfgFile, []byte("port: [invalid yaml\n"), 0644)

	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	// File exists but is malformed — must still error
	if err := Load(c, WithOptionalConfigFile(cfgFile), WithoutFlags()); err == nil {
		t.Fatal("expected error for malformed optional config file")
	}
}

// --- MustLoad ---

func TestMustLoadSuccess(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	// Should not panic
	MustLoad(c, WithoutFlags())
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080", c.Port)
	}
}

func TestMustLoadPanics(t *testing.T) {
	type cfg struct {
		Token string `yaml:"token" required:"true"`
	}
	c := &cfg{}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustLoad to panic on error")
		}
	}()
	MustLoad(c, WithoutFlags())
}

// --- Reload ---

func TestReload(t *testing.T) {
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

	// Change the file and reload manually
	os.WriteFile(cfgFile, []byte("port: 9090\n"), 0644)
	if err := loader.Reload(); err != nil {
		t.Fatalf("Reload error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090 after Reload", c.Port)
	}
}

func TestReloadBeforeLoad(t *testing.T) {
	loader := NewLoader(WithoutFlags())
	if err := loader.Reload(); err == nil {
		t.Fatal("expected error when Reload called before Load")
	}
}

func TestReloadValidationError(t *testing.T) {
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

	// Write invalid value then reload
	os.WriteFile(cfgFile, []byte("port: 99999\n"), 0644)
	if err := loader.Reload(); err == nil {
		t.Fatal("expected validation error from Reload")
	}
	// cfg retains previous valid value
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080 (unchanged on validation error)", c.Port)
	}
}

// --- WithConfigFlag ---

func TestConfigFlag(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgFile, []byte("port: 9090\n"), 0644)

	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	if err := Load(c, WithConfigFlag("config"), WithArgs([]string{"--config", cfgFile})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090", c.Port)
	}
}

func TestConfigFlagOverridesExplicit(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	override := filepath.Join(dir, "override.yaml")
	os.WriteFile(base, []byte("port: 7070\n"), 0644)
	os.WriteFile(override, []byte("port: 9090\n"), 0644)

	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	err := Load(c,
		WithConfigFile(base),
		WithConfigFlag("config"),
		WithArgs([]string{"--config", override}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// --config file has higher precedence than WithConfigFile
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090 (--config should override WithConfigFile)", c.Port)
	}
}

func TestConfigFlagMultiple(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	os.WriteFile(first, []byte("port: 7070\nhost: first\n"), 0644)
	os.WriteFile(second, []byte("port: 9090\n"), 0644)

	type cfg struct {
		Port int    `yaml:"port" default:"8080"`
		Host string `yaml:"host" default:"localhost"`
	}
	c := &cfg{}
	err := Load(c,
		WithConfigFlag("config"),
		WithArgs([]string{"--config", first, "--config", second}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// second has higher precedence for port
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090 (second --config wins)", c.Port)
	}
	// host only in first, preserved
	if c.Host != "first" {
		t.Errorf("Host = %q, want %q", c.Host, "first")
	}
}

func TestConfigFlagURLScheme(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	remote := newFakeRemote(map[string]any{"port": 9090})

	c := &cfg{}
	err := Load(c,
		WithConfigFlag("config"),
		WithURLScheme("fake", func(rawURL string) (RemoteProvider, error) {
			return remote, nil
		}),
		WithArgs([]string{"--config", "fake://host/key"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090 (from URL scheme resolver)", c.Port)
	}
}

func TestConfigFlagUnknownScheme(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	c := &cfg{}
	err := Load(c,
		WithConfigFlag("config"),
		WithArgs([]string{"--config", "etcd://host/key"}),
	)
	if err == nil {
		t.Fatal("expected error for unregistered scheme")
	}
	if !strings.Contains(err.Error(), "etcd") {
		t.Errorf("error = %q, want it to mention the scheme", err.Error())
	}
}

// --- WithExtraFlags ---

func TestExtraFlags(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	var name string
	c := &cfg{}
	err := Load(c,
		WithExtraFlags(func(fs *pflag.FlagSet) {
			fs.StringVar(&name, "name", "default", "app name")
		}),
		WithArgs([]string{"--name", "myapp"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "myapp" {
		t.Errorf("name = %q, want %q", name, "myapp")
	}
}

func TestExtraFlagsWithURLScheme(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	// token is captured by the resolver closure; populated from --config-token flag
	var token string
	var capturedToken string

	c := &cfg{}
	remote := newFakeRemote(map[string]any{"port": 9090})

	err := Load(c,
		WithConfigFlag("config"),
		WithExtraFlags(func(fs *pflag.FlagSet) {
			fs.StringVar(&token, "config-token", "", "bearer token")
		}),
		WithURLScheme("fake", func(rawURL string) (RemoteProvider, error) {
			capturedToken = token // token is populated by the time this runs
			return remote, nil
		}),
		WithArgs([]string{"--config", "fake://host/key", "--config-token", "mybearer"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090", c.Port)
	}
	if capturedToken != "mybearer" {
		t.Errorf("capturedToken = %q, want %q (token should be set before resolver runs)", capturedToken, "mybearer")
	}
}

// --- Remote Config ---

func TestRemoteSource(t *testing.T) {
	type cfg struct {
		Port int    `yaml:"port" default:"8080"`
		Host string `yaml:"host" default:"localhost"`
	}
	remote := newFakeRemote(map[string]any{"port": 9090, "host": "remote.host"})
	c := &cfg{}
	if err := Load(c, WithRemote(remote), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090", c.Port)
	}
	if c.Host != "remote.host" {
		t.Errorf("Host = %q, want %q", c.Host, "remote.host")
	}
}

func TestRemoteOverridesFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgFile, []byte("port: 7070\nhost: file.host\n"), 0644)

	type cfg struct {
		Port int    `yaml:"port" default:"8080"`
		Host string `yaml:"host" default:"localhost"`
	}
	// file registered first (lower precedence), remote registered second (higher precedence)
	remote := newFakeRemote(map[string]any{"port": 9090})
	c := &cfg{}
	if err := Load(c, WithConfigFile(cfgFile), WithRemote(remote), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090 (remote should override file)", c.Port)
	}
	// host only set in file, not in remote — file value preserved
	if c.Host != "file.host" {
		t.Errorf("Host = %q, want %q (file value when remote doesn't set it)", c.Host, "file.host")
	}
}

func TestFileOverridesRemote(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgFile, []byte("port: 7070\n"), 0644)

	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	// remote registered first (lower precedence), file registered second (higher precedence)
	remote := newFakeRemote(map[string]any{"port": 9090})
	c := &cfg{}
	if err := Load(c, WithRemote(remote), WithConfigFile(cfgFile), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 7070 {
		t.Errorf("Port = %d, want 7070 (file should override remote)", c.Port)
	}
}

func TestLoaderSourceRemote(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	remote := newFakeRemote(map[string]any{"port": 9090})
	c := &cfg{}
	loader := NewLoader(WithRemote(remote), WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	loader.Print(&buf)
	if !strings.Contains(buf.String(), "remote") {
		t.Errorf("expected source 'remote' in output:\n%s", buf.String())
	}
}

func TestLoaderWatchRemote(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}
	remote := newFakeRemote(map[string]any{"port": 9090})
	c := &cfg{}
	loader := NewLoader(WithRemote(remote), WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}
	if c.Port != 9090 {
		t.Fatalf("Port = %d, want 9090", c.Port)
	}

	done := make(chan error, 1)
	if err := loader.Watch(c, func(err error) {
		done <- err
	}); err != nil {
		t.Fatal(err)
	}

	remote.update(map[string]any{"port": 3000})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Watch callback error: %v", err)
		}
		if c.Port != 3000 {
			t.Errorf("Port = %d, want 3000 after reload", c.Port)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Watch callback not called within 3 seconds")
	}
}

func TestLoaderWatchRemoteValidationError(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080" validate:"min=1,max=65535"`
	}
	remote := newFakeRemote(map[string]any{"port": 9090})
	c := &cfg{}
	loader := NewLoader(WithRemote(remote), WithoutFlags())
	if err := loader.Load(c); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	if err := loader.Watch(c, func(err error) {
		done <- err
	}); err != nil {
		t.Fatal(err)
	}

	remote.update(map[string]any{"port": 99999})

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected validation error from Watch")
		}
		// cfg should retain old value on error
		if c.Port != 9090 {
			t.Errorf("Port = %d, want 9090 (unchanged on validation error)", c.Port)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Watch callback not called within 3 seconds")
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
