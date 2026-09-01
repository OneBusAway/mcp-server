package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.json")
	envPath := filepath.Join(directory, ".env")
	writeTestFile(t, configPath, `{
  "upstream": {"base-url": "http://json.example", "api-key": "json-key"},
  "mcp": {"transport": "http", "tool-profile": "rider"},
  "http": {
    "bind-address": "127.0.0.1",
    "port": 8080,
    "auth-token": "json-token",
    "allowed-origins": ["https://json.example"]
  },
  "logging": {"path": "/tmp/json.log", "format": "text"},
  "cache": {"path": "/tmp/json.db"}
}`)
	writeTestFile(t, envPath, strings.Join([]string{
		"OBA_API_KEY=dotenv-key",
		"OBA_HTTP_AUTH_TOKEN=dotenv-token",
		"OBA_PORT=9090",
		"OBA_LOG_JSON=1",
	}, "\n"))

	cfg, err := Load(LoadOptions{
		ConfigFile: configPath,
		EnvFile:    envPath,
		LookupEnv: mapLookup(map[string]string{
			"OBA_PORT":         "8081",
			"OBA_TOOL_PROFILE": "all",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upstream.BaseURL != "http://json.example" {
		t.Fatalf("base URL = %q, want JSON value", cfg.Upstream.BaseURL)
	}
	if cfg.Upstream.APIKey != "dotenv-key" || cfg.HTTP.AuthToken != "dotenv-token" {
		t.Fatal("dotenv secrets did not override JSON")
	}
	if cfg.HTTP.Port != 8081 || cfg.MCP.ToolProfile != "all" {
		t.Fatal("process environment did not override lower-priority sources")
	}
	if cfg.Logging.Format != "json" {
		t.Fatalf("logging format = %q, want json", cfg.Logging.Format)
	}
	if cfg.MCP.Transport != TransportStreamableHTTP {
		t.Fatalf("transport = %q, want canonical streamable-http", cfg.MCP.Transport)
	}
	if cfg.sources[fieldBaseURL] != sourceJSON || cfg.sources[fieldAPIKey] != sourceDotenv || cfg.sources[fieldPort] != sourceEnvironment {
		t.Fatalf("sources were not preserved: %#v", cfg.sources)
	}
}

func TestLoadSupportsEnvironmentOnly(t *testing.T) {
	cfg, err := Load(LoadOptions{LookupEnv: mapLookup(map[string]string{
		"OBA_API_KEY": "test-key",
	})})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upstream.BaseURL != "http://localhost:4000" || cfg.MCP.Transport != "stdio" || cfg.HTTP.Port != 8080 {
		t.Fatalf("defaults were not preserved: %#v", cfg)
	}
}

func TestLoadSupportsDotenvOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeTestFile(t, path, "OBA_API_KEY=dotenv-key\nOBA_BASE_URL=https://oba.example\n")
	cfg, err := Load(LoadOptions{EnvFile: path, LookupEnv: mapLookup(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upstream.APIKey != "dotenv-key" || cfg.Upstream.BaseURL != "https://oba.example" {
		t.Fatalf("dotenv values not loaded: %#v", cfg.Upstream)
	}
}

func TestLoadExplicitEmptyEnvironmentSecretDoesNotFallBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, `{"upstream":{"api-key":"json-key"}}`)
	_, err := Load(LoadOptions{
		ConfigFile: path,
		LookupEnv:  mapLookup(map[string]string{"OBA_API_KEY": ""}),
	})
	if err == nil || !strings.Contains(err.Error(), "upstream.api-key is required") {
		t.Fatalf("error = %v, want required API key failure", err)
	}
}

func TestResolveReturnsConfigurationBeforeSemanticValidation(t *testing.T) {
	cfg, err := Resolve(LoadOptions{LookupEnv: mapLookup(map[string]string{
		"OBA_API_KEY": "",
	})})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upstream.APIKey != "" || cfg.sources[fieldAPIKey] != sourceEnvironment {
		t.Fatalf("resolved API key = %q from %q", cfg.Upstream.APIKey, cfg.sources[fieldAPIKey])
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "upstream.api-key is required") {
		t.Fatalf("validation error = %v, want required API key failure", err)
	}
}

func TestLoadRejectsUnknownJSONField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, `{"upstream":{"api-key":"test"},"unknown":true}`)
	_, err := Load(LoadOptions{ConfigFile: path, LookupEnv: mapLookup(nil)})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field failure", err)
	}
}

func TestLoadRejectsTrailingJSONDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, `{"upstream":{"api-key":"test"}} {}`)
	_, err := Load(LoadOptions{ConfigFile: path, LookupEnv: mapLookup(nil)})
	if err == nil || !strings.Contains(err.Error(), "multiple JSON documents") {
		t.Fatalf("error = %v, want trailing document failure", err)
	}
}

func TestLoadRejectsJSONNull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, `{"upstream":{"api-key":null}}`)
	_, err := Load(LoadOptions{ConfigFile: path, LookupEnv: mapLookup(nil)})
	if err == nil || !strings.Contains(err.Error(), "configuration.upstream.api-key must not be null") {
		t.Fatalf("error = %v, want null field failure", err)
	}
}

func TestLoadRejectsInvalidEnvironmentValues(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "port", values: map[string]string{"OBA_API_KEY": "test", "OBA_PORT": "eight"}, want: "OBA_PORT must be an integer"},
		{name: "log boolean", values: map[string]string{"OBA_API_KEY": "test", "OBA_LOG_JSON": "sometimes"}, want: "OBA_LOG_JSON"},
		{name: "origin", values: map[string]string{"OBA_API_KEY": "test", "OBA_ALLOWED_ORIGINS": "https://safe.example,"}, want: "empty origin"},
		{name: "conflicting log formats", values: map[string]string{"OBA_API_KEY": "test", "OBA_LOG_FORMAT": "text", "OBA_LOG_JSON": "1"}, want: "conflicts"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(LoadOptions{LookupEnv: mapLookup(test.values)})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateHTTPRequirements(t *testing.T) {
	cfg := Defaults()
	cfg.Upstream.APIKey = "test"
	cfg.MCP.Transport = TransportStreamableHTTP
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "http.auth-token") {
		t.Fatalf("error = %v, want HTTP token failure", err)
	}
	cfg.HTTP.AuthToken = "secret"
	cfg.HTTP.AllowedOrigins = []string{"https://app.example", "https://app.example"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.HTTP.AllowedOrigins, []string{"https://app.example"}) {
		t.Fatalf("origins = %#v, want deduplicated list", cfg.HTTP.AllowedOrigins)
	}
}

func TestLoadCanonicalLogFormat(t *testing.T) {
	cfg, err := Load(LoadOptions{LookupEnv: mapLookup(map[string]string{
		"OBA_API_KEY":    "test",
		"OBA_LOG_FORMAT": "json",
	})})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.Format != "json" || cfg.sources[fieldLogFormat] != sourceEnvironment {
		t.Fatalf("logging configuration = %q from %q", cfg.Logging.Format, cfg.sources[fieldLogFormat])
	}
}

func TestLoadRejectsExplicitHTTPSettingsForStdio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, `{"upstream":{"api-key":"test"},"http":{"port":8080}}`)
	_, err := Load(LoadOptions{ConfigFile: path, LookupEnv: mapLookup(nil)})
	if err == nil || !strings.Contains(err.Error(), "http.port is HTTP-only") {
		t.Fatalf("error = %v, want contradictory HTTP configuration failure", err)
	}
}

func TestLoadRejectsExplicitEmptyAllowedOriginsForStdio(t *testing.T) {
	_, err := Load(LoadOptions{LookupEnv: mapLookup(map[string]string{
		"OBA_API_KEY":         "test",
		"OBA_ALLOWED_ORIGINS": "",
	})})
	if err == nil || !strings.Contains(err.Error(), "http.allowed-origins is HTTP-only") {
		t.Fatalf("error = %v, want explicit empty HTTP-only configuration failure", err)
	}
}

func TestLoadAllowsExplicitExternalBindForStreamableHTTP(t *testing.T) {
	cfg, err := Load(LoadOptions{LookupEnv: mapLookup(map[string]string{
		"OBA_API_KEY":         "test",
		"OBA_TRANSPORT":       TransportStreamableHTTP,
		"OBA_HTTP_AUTH_TOKEN": "secret",
		"OBA_HTTP_BIND_ADDR":  "0.0.0.0",
	})})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.BindAddress != "0.0.0.0" {
		t.Fatalf("bind address = %q", cfg.HTTP.BindAddress)
	}
}

func TestEmptyCachePathDisablesPersistentCache(t *testing.T) {
	cfg, err := Load(LoadOptions{LookupEnv: mapLookup(map[string]string{
		"OBA_API_KEY": "test",
		"OBA_CACHE":   "",
	})})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cache.Path != "" {
		t.Fatalf("cache path = %q, want disabled persistent cache", cfg.Cache.Path)
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
		ok   bool
	}{
		{name: "empty disables browser access", raw: "", want: []string{}, ok: true},
		{name: "deduplicates valid origins", raw: "https://app.example, http://localhost:3000, https://app.example", want: []string{"https://app.example", "http://localhost:3000"}, ok: true},
		{name: "rejects wildcard", raw: "*"},
		{name: "rejects path", raw: "https://app.example/mcp"},
		{name: "rejects query", raw: "https://app.example?x=1"},
		{name: "rejects unsupported scheme", raw: "ftp://app.example"},
		{name: "rejects empty member", raw: "https://app.example,"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseAllowedOrigins(test.raw)
			if (err == nil) != test.ok {
				t.Fatalf("error = %v, want success %t", err, test.ok)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestRedactedJSONDoesNotExposeSecrets(t *testing.T) {
	cfg := Defaults()
	cfg.Upstream.APIKey = "upstream-secret"
	cfg.HTTP.AuthToken = "http-secret"
	output, err := cfg.RedactedJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	if strings.Contains(text, "upstream-secret") || strings.Contains(text, "http-secret") {
		t.Fatalf("redacted output contains a secret: %s", text)
	}
	if strings.Count(text, RedactedValue) != 2 {
		t.Fatalf("redacted output = %s", text)
	}
	if !strings.Contains(text, `"upstream.api-key": "default"`) {
		t.Fatalf("redacted output does not include sources: %s", text)
	}
}

func TestLoadAllowsExplicitSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.json")
	link := filepath.Join(directory, "config.json")
	writeTestFile(t, target, `{"upstream":{"api-key":"test"}}`)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(LoadOptions{ConfigFile: link, LookupEnv: mapLookup(nil)}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsMissingExplicitFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	_, err := Load(LoadOptions{ConfigFile: path, LookupEnv: mapLookup(nil)})
	if err == nil || !strings.Contains(err.Error(), "load config file") {
		t.Fatalf("error = %v, want missing config failure", err)
	}
}

func TestLoadRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxConfigFileBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Load(LoadOptions{ConfigFile: path, LookupEnv: mapLookup(nil)})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want size-limit failure", err)
	}
}

func TestMalformedDotenvErrorDoesNotExposeLineContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeTestFile(t, path, "OBA_API_KEY='do-not-expose\n")
	_, err := Load(LoadOptions{EnvFile: path, LookupEnv: mapLookup(nil)})
	if err == nil || !strings.Contains(err.Error(), "invalid dotenv syntax") {
		t.Fatalf("error = %v, want dotenv syntax failure", err)
	}
	if strings.Contains(err.Error(), "do-not-expose") {
		t.Fatalf("error exposes dotenv contents: %v", err)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
