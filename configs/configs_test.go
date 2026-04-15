package configs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	type cfg struct {
		Port  int    `yaml:"port" default:"8080"`
		Host  string `yaml:"host" default:"localhost"`
		Debug bool   `yaml:"debug" default:"false"`
	}

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080", c.Port)
	}
	if c.Host != "localhost" {
		t.Errorf("Host = %q, want %q", c.Host, "localhost")
	}
	if c.Debug {
		t.Errorf("Debug = true, want false")
	}
}

func TestLoadFromYAML(t *testing.T) {
	yamlContent := `
port: 9090
host: 0.0.0.0
database:
  host: db.example.com
  port: 3306
  name: mydb
`
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	type dbCfg struct {
		Host string `yaml:"host" default:"localhost"`
		Port int    `yaml:"port" default:"5432"`
		Name string `yaml:"name"`
	}
	type cfg struct {
		Port     int    `yaml:"port" default:"8080"`
		Host     string `yaml:"host" default:"localhost"`
		Database dbCfg  `yaml:"database"`
	}

	c := &cfg{}
	if err := Load(c, WithConfigFile(cfgFile), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 9090 {
		t.Errorf("Port = %d, want 9090", c.Port)
	}
	if c.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", c.Host, "0.0.0.0")
	}
	if c.Database.Host != "db.example.com" {
		t.Errorf("Database.Host = %q, want %q", c.Database.Host, "db.example.com")
	}
	if c.Database.Port != 3306 {
		t.Errorf("Database.Port = %d, want 3306", c.Database.Port)
	}
	if c.Database.Name != "mydb" {
		t.Errorf("Database.Name = %q, want %q", c.Database.Name, "mydb")
	}
}

func TestLoadFromEnv(t *testing.T) {
	type cfg struct {
		Port int    `yaml:"port" default:"8080"`
		Host string `yaml:"host" default:"localhost"`
	}

	t.Setenv("APP_PORT", "3000")
	t.Setenv("APP_HOST", "envhost")

	c := &cfg{}
	if err := Load(c, WithEnvPrefix("APP"), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 3000 {
		t.Errorf("Port = %d, want 3000", c.Port)
	}
	if c.Host != "envhost" {
		t.Errorf("Host = %q, want %q", c.Host, "envhost")
	}
}

func TestLoadFromEnvNested(t *testing.T) {
	type cfg struct {
		Database struct {
			Host string `yaml:"host" default:"localhost"`
			Port int    `yaml:"port" default:"5432"`
		} `yaml:"database"`
	}

	t.Setenv("DATABASE_HOST", "envdb")
	t.Setenv("DATABASE_PORT", "3307")

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Database.Host != "envdb" {
		t.Errorf("Database.Host = %q, want %q", c.Database.Host, "envdb")
	}
	if c.Database.Port != 3307 {
		t.Errorf("Database.Port = %d, want 3307", c.Database.Port)
	}
}

func TestLoadFromEnvNestedWithPrefix(t *testing.T) {
	type cfg struct {
		Database struct {
			Host string `yaml:"host" default:"localhost"`
		} `yaml:"database"`
	}

	t.Setenv("MYAPP_DATABASE_HOST", "prefixed")

	c := &cfg{}
	if err := Load(c, WithEnvPrefix("MYAPP"), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Database.Host != "prefixed" {
		t.Errorf("Database.Host = %q, want %q", c.Database.Host, "prefixed")
	}
}

func TestLoadFromFlags(t *testing.T) {
	type cfg struct {
		Port int    `yaml:"port" default:"8080"`
		Host string `yaml:"host" default:"localhost"`
	}

	c := &cfg{}
	if err := Load(c, WithArgs([]string{"--port", "4000", "--host", "flaghost"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 4000 {
		t.Errorf("Port = %d, want 4000", c.Port)
	}
	if c.Host != "flaghost" {
		t.Errorf("Host = %q, want %q", c.Host, "flaghost")
	}
}

func TestLoadFromFlagsNested(t *testing.T) {
	type cfg struct {
		Database struct {
			Host string `yaml:"host" default:"localhost"`
			Port int    `yaml:"port" default:"5432"`
		} `yaml:"database"`
	}

	c := &cfg{}
	if err := Load(c, WithArgs([]string{"--database-host", "flagdb", "--database-port", "3308"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Database.Host != "flagdb" {
		t.Errorf("Database.Host = %q, want %q", c.Database.Host, "flagdb")
	}
	if c.Database.Port != 3308 {
		t.Errorf("Database.Port = %d, want 3308", c.Database.Port)
	}
}

func TestPrecedenceFlagOverEnvOverYAMLOverDefault(t *testing.T) {
	yamlContent := `
port: 9090
host: yamlhost
log_level: yamllevel
`
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	type cfg struct {
		Port     int    `yaml:"port" default:"8080"`
		Host     string `yaml:"host" default:"localhost"`
		LogLevel string `yaml:"log_level" default:"info"`
		Extra    string `yaml:"extra" default:"defaultval"`
	}

	t.Setenv("APP_PORT", "3000")
	t.Setenv("APP_HOST", "envhost")

	c := &cfg{}
	err := Load(c,
		WithConfigFile(cfgFile),
		WithEnvPrefix("APP"),
		WithArgs([]string{"--host", "flaghost"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// port: env=3000 > yaml=9090 > default=8080
	if c.Port != 3000 {
		t.Errorf("Port = %d, want 3000 (env wins over yaml)", c.Port)
	}
	// host: flag=flaghost > env=envhost > yaml=yamlhost > default=localhost
	if c.Host != "flaghost" {
		t.Errorf("Host = %q, want %q (flag wins over env)", c.Host, "flaghost")
	}
	// log_level: yaml=yamllevel > default=info (no flag, no env)
	if c.LogLevel != "yamllevel" {
		t.Errorf("LogLevel = %q, want %q (yaml wins over default)", c.LogLevel, "yamllevel")
	}
	// extra: only default
	if c.Extra != "defaultval" {
		t.Errorf("Extra = %q, want %q (default)", c.Extra, "defaultval")
	}
}

func TestRequiredFieldMissing(t *testing.T) {
	type cfg struct {
		Name string `yaml:"name" required:"true"`
	}

	c := &cfg{}
	err := Load(c, WithoutFlags())
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
}

func TestRequiredFieldProvided(t *testing.T) {
	type cfg struct {
		Name string `yaml:"name" required:"true"`
	}

	t.Setenv("NAME", "provided")

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Name != "provided" {
		t.Errorf("Name = %q, want %q", c.Name, "provided")
	}
}

func TestRequiredFieldWithDefault(t *testing.T) {
	type cfg struct {
		Name string `yaml:"name" required:"true" default:"fallback"`
	}

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Name != "fallback" {
		t.Errorf("Name = %q, want %q", c.Name, "fallback")
	}
}

func TestDuration(t *testing.T) {
	type cfg struct {
		Timeout time.Duration `yaml:"timeout" default:"30s"`
	}

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", c.Timeout)
	}
}

func TestDurationFromFlag(t *testing.T) {
	type cfg struct {
		Timeout time.Duration `yaml:"timeout" default:"30s"`
	}

	c := &cfg{}
	if err := Load(c, WithArgs([]string{"--timeout", "5m"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Timeout != 5*time.Minute {
		t.Errorf("Timeout = %v, want 5m", c.Timeout)
	}
}

func TestBoolFlag(t *testing.T) {
	type cfg struct {
		Debug bool `yaml:"debug" default:"false"`
	}

	c := &cfg{}
	if err := Load(c, WithArgs([]string{"--debug"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Debug {
		t.Errorf("Debug = false, want true")
	}
}

func TestFloat64(t *testing.T) {
	type cfg struct {
		Rate float64 `yaml:"rate" default:"0.5"`
	}

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Rate != 0.5 {
		t.Errorf("Rate = %f, want 0.5", c.Rate)
	}
}

func TestStringSliceFromYAML(t *testing.T) {
	yamlContent := `
tags:
  - alpha
  - beta
`
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	type cfg struct {
		Tags []string `yaml:"tags"`
	}

	c := &cfg{}
	if err := Load(c, WithConfigFile(cfgFile), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Tags) != 2 || c.Tags[0] != "alpha" || c.Tags[1] != "beta" {
		t.Errorf("Tags = %v, want [alpha beta]", c.Tags)
	}
}

func TestAutoSnakeCase(t *testing.T) {
	type cfg struct {
		LogLevel string `default:"info"`
	}

	t.Setenv("LOG_LEVEL", "debug")

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", c.LogLevel, "debug")
	}
}

func TestCustomEnvTag(t *testing.T) {
	type cfg struct {
		Secret string `yaml:"secret" env:"MY_CUSTOM_SECRET"`
	}

	t.Setenv("MY_CUSTOM_SECRET", "s3cret")

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Secret != "s3cret" {
		t.Errorf("Secret = %q, want %q", c.Secret, "s3cret")
	}
}

func TestCustomFlagTag(t *testing.T) {
	type cfg struct {
		Verbose bool `yaml:"verbose" flag:"v" default:"false"`
	}

	c := &cfg{}
	if err := Load(c, WithArgs([]string{"-v"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Verbose {
		t.Error("Verbose = false, want true")
	}
}

func TestNonPointerError(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port"`
	}

	err := Load(cfg{}, WithoutFlags())
	if err == nil {
		t.Fatal("expected error for non-pointer")
	}
}

func TestConfigFileNotFound(t *testing.T) {
	type cfg struct {
		Port int `yaml:"port" default:"8080"`
	}

	c := &cfg{}
	err := Load(c, WithConfigFile("/nonexistent/config.yaml"), WithoutFlags())
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestAnonymousEmbed(t *testing.T) {
	type Base struct {
		Host string `yaml:"host" default:"localhost"`
		Port int    `yaml:"port" default:"8080"`
	}
	type cfg struct {
		Base
		Name string `yaml:"name" default:"app"`
	}

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Host != "localhost" {
		t.Errorf("Host = %q, want %q", c.Host, "localhost")
	}
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080", c.Port)
	}
	if c.Name != "app" {
		t.Errorf("Name = %q, want %q", c.Name, "app")
	}
}

func TestYAMLSkipField(t *testing.T) {
	type cfg struct {
		Port    int    `yaml:"port" default:"8080"`
		Ignored string `yaml:"-"`
	}

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080", c.Port)
	}
}

func TestDeepNesting(t *testing.T) {
	type cfg struct {
		Server struct {
			HTTP struct {
				Port int `yaml:"port" default:"8080"`
			} `yaml:"http"`
		} `yaml:"server"`
	}

	t.Setenv("SERVER_HTTP_PORT", "9999")

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Server.HTTP.Port != 9999 {
		t.Errorf("Server.HTTP.Port = %d, want 9999", c.Server.HTTP.Port)
	}
}

func TestDeepNestingFlag(t *testing.T) {
	type cfg struct {
		Server struct {
			HTTP struct {
				Port int `yaml:"port" default:"8080"`
			} `yaml:"http"`
		} `yaml:"server"`
	}

	c := &cfg{}
	if err := Load(c, WithArgs([]string{"--server-http-port", "7777"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Server.HTTP.Port != 7777 {
		t.Errorf("Server.HTTP.Port = %d, want 7777", c.Server.HTTP.Port)
	}
}

func TestMultipleRequiredMissing(t *testing.T) {
	type cfg struct {
		A string `yaml:"a" required:"true"`
		B string `yaml:"b" required:"true"`
	}

	c := &cfg{}
	err := Load(c, WithoutFlags())
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
	if got := err.Error(); !contains(got, "a") || !contains(got, "b") {
		t.Errorf("error = %q, want it to mention both 'a' and 'b'", got)
	}
}

func TestNestedEnvTagOverride(t *testing.T) {
	type cfg struct {
		Database struct {
			Password string `yaml:"password" env:"SECRET"`
		} `yaml:"database"`
	}

	// env tag "SECRET" only replaces the leaf; parent "DATABASE" is still prepended
	t.Setenv("DATABASE_SECRET", "s3cret")

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Database.Password != "s3cret" {
		t.Errorf("Database.Password = %q, want %q", c.Database.Password, "s3cret")
	}
}

func TestNestedEnvTagOverrideWithPrefix(t *testing.T) {
	type cfg struct {
		Database struct {
			Password string `yaml:"password" env:"SECRET"`
		} `yaml:"database"`
	}

	t.Setenv("APP_DATABASE_SECRET", "s3cret")

	c := &cfg{}
	if err := Load(c, WithEnvPrefix("APP"), WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Database.Password != "s3cret" {
		t.Errorf("Database.Password = %q, want %q", c.Database.Password, "s3cret")
	}
}

func TestNestedFlagTagOverride(t *testing.T) {
	type cfg struct {
		Database struct {
			Password string `yaml:"password" flag:"secret"`
		} `yaml:"database"`
	}

	// flag tag "secret" only replaces the leaf; parent "database" is still prepended
	c := &cfg{}
	if err := Load(c, WithArgs([]string{"--database-secret", "flagpw"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Database.Password != "flagpw" {
		t.Errorf("Database.Password = %q, want %q", c.Database.Password, "flagpw")
	}
}

func TestDeepNestedOverrides(t *testing.T) {
	type cfg struct {
		Server struct {
			HTTP struct {
				Port int `yaml:"port" env:"LISTEN_PORT" flag:"listen-port" default:"8080"`
			} `yaml:"http"`
		} `yaml:"server"`
	}

	// env override: SERVER_HTTP_ prefix stays, leaf becomes LISTEN_PORT
	t.Setenv("SERVER_HTTP_LISTEN_PORT", "1234")

	c := &cfg{}
	if err := Load(c, WithoutFlags()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Server.HTTP.Port != 1234 {
		t.Errorf("Server.HTTP.Port = %d, want 1234", c.Server.HTTP.Port)
	}

	// flag override: server-http- prefix stays, leaf becomes listen-port
	c2 := &cfg{}
	if err := Load(c2, WithArgs([]string{"--server-http-listen-port", "5678"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c2.Server.HTTP.Port != 5678 {
		t.Errorf("Server.HTTP.Port = %d, want 5678", c2.Server.HTTP.Port)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
