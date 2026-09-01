package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestLoadStartupConfigCLISelectorsOverrideEnvironmentSelectors(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.json")
	envPath := filepath.Join(directory, ".env")
	writeStartupTestFile(t, configPath, `{"upstream":{"base-url":"https://configured.example"},"mcp":{"transport":"streamable-http"}}`)
	writeStartupTestFile(t, envPath, "OBA_PORT=9090\nOBA_HTTP_AUTH_TOKEN=dotenv-token\n")

	values := map[string]string{
		"OBA_CONFIG_FILE": filepath.Join(directory, "wrong.json"),
		"OBA_ENV_FILE":    filepath.Join(directory, "wrong.env"),
		"OBA_API_KEY":     "process-key",
	}
	cfg, check, print, err := loadStartupConfig(
		[]string{"--config", configPath, "--env-file", envPath, "--check-config"},
		func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !check || print {
		t.Fatalf("check = %t, print = %t", check, print)
	}
	if cfg.Upstream.BaseURL != "https://configured.example" || cfg.HTTP.Port != 9090 {
		t.Fatalf("CLI-selected files were not loaded: %#v", cfg)
	}
}

func TestConfigExampleMatchesSchema(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile("./config.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open("./config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := jsonschema.UnmarshalJSON(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatal(err)
	}
}

func TestLoadStartupConfigRejectsUnexpectedArguments(t *testing.T) {
	_, _, _, err := loadStartupConfig([]string{"unexpected"}, func(key string) (string, bool) {
		return map[string]string{"OBA_API_KEY": "test"}[key], key == "OBA_API_KEY"
	})
	if err == nil {
		t.Fatal("unexpected positional argument was accepted")
	}
}

func TestLoadStartupConfigStreamableHTTP(t *testing.T) {
	values := map[string]string{
		"OBA_API_KEY":         "test",
		"OBA_TRANSPORT":       "streamable-http",
		"OBA_HTTP_AUTH_TOKEN": "secret",
		"OBA_ALLOWED_ORIGINS": "https://app.example",
	}
	cfg, _, _, err := loadStartupConfig(nil, func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCP.Transport != "streamable-http" || len(cfg.HTTP.AllowedOrigins) != 1 {
		t.Fatalf("streamable HTTP configuration = %#v", cfg)
	}
}

func TestResolveStartupConfigReturnsInvalidConfigurationForPrint(t *testing.T) {
	cfg, check, print, err := resolveStartupConfig([]string{"--print-config"}, func(key string) (string, bool) {
		return map[string]string{"OBA_API_KEY": ""}[key], key == "OBA_API_KEY"
	})
	if err != nil {
		t.Fatal(err)
	}
	if check || !print {
		t.Fatalf("check = %t, print = %t", check, print)
	}
	if cfg.Upstream.APIKey != "" {
		t.Fatalf("resolved API key = %q, want explicit empty value", cfg.Upstream.APIKey)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid configuration was accepted")
	}
}

func TestPrepareLogPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "oba-mcp.log")
	if err := prepareLogPath(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareLogPathReportsFilesystemFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	writeStartupTestFile(t, blocker, "file")
	if err := prepareLogPath(filepath.Join(blocker, "oba-mcp.log")); err == nil {
		t.Fatal("invalid log directory was accepted")
	}
}

func TestOpenPersistentCacheEmptyUsesMemoryOnly(t *testing.T) {
	queries, database, err := openPersistentCache("")
	if err != nil {
		t.Fatal(err)
	}
	if queries != nil || database != nil {
		t.Fatal("empty cache path opened persistent storage")
	}
}

func TestOpenPersistentCacheReportsFilesystemFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	writeStartupTestFile(t, blocker, "file")
	path := filepath.Join(blocker, "cache.db")
	if _, _, err := openPersistentCache(path); err == nil {
		t.Fatal("invalid cache directory was accepted")
	}
}

func writeStartupTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
