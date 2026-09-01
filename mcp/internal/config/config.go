// Package config resolves and validates oba-mcp runtime configuration.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const RedactedValue = "[REDACTED]"

const (
	TransportStdio          = "stdio"
	TransportStreamableHTTP = "streamable-http"
)

type source string

const (
	sourceDefault     source = "default"
	sourceJSON        source = "config.json"
	sourceDotenv      source = ".env"
	sourceEnvironment source = "process-environment"
)

type field string

const (
	fieldBaseURL        field = "upstream.base-url"
	fieldAPIKey         field = "upstream.api-key"
	fieldTransport      field = "mcp.transport"
	fieldToolProfile    field = "mcp.tool-profile"
	fieldBindAddress    field = "http.bind-address"
	fieldPort           field = "http.port"
	fieldAuthToken      field = "http.auth-token"
	fieldAllowedOrigins field = "http.allowed-origins"
	fieldLogPath        field = "logging.path"
	fieldLogFormat      field = "logging.format"
	fieldCachePath      field = "cache.path"
)

var configuredFields = []field{
	fieldBaseURL,
	fieldAPIKey,
	fieldTransport,
	fieldToolProfile,
	fieldBindAddress,
	fieldPort,
	fieldAuthToken,
	fieldAllowedOrigins,
	fieldLogPath,
	fieldLogFormat,
	fieldCachePath,
}

// Config is the resolved runtime configuration used by the MCP server.
type Config struct {
	Schema   string         `json:"$schema,omitempty"`
	Upstream UpstreamConfig `json:"upstream"`
	MCP      MCPConfig      `json:"mcp"`
	HTTP     HTTPConfig     `json:"http"`
	Logging  LoggingConfig  `json:"logging"`
	Cache    CacheConfig    `json:"cache"`

	sources map[field]source
}

type UpstreamConfig struct {
	BaseURL string `json:"base-url"`
	APIKey  string `json:"api-key,omitempty"`
}

type MCPConfig struct {
	Transport   string `json:"transport"`
	ToolProfile string `json:"tool-profile"`
}

type HTTPConfig struct {
	BindAddress    string   `json:"bind-address"`
	Port           int      `json:"port"`
	AuthToken      string   `json:"auth-token,omitempty"`
	AllowedOrigins []string `json:"allowed-origins"`
}

type LoggingConfig struct {
	Path   string `json:"path"`
	Format string `json:"format"`
}

type CacheConfig struct {
	Path string `json:"path"`
}

// Defaults returns the configuration used when no source overrides a field.
func Defaults() Config {
	cfg := Config{
		Upstream: UpstreamConfig{BaseURL: "http://localhost:4000"},
		MCP: MCPConfig{
			Transport:   TransportStdio,
			ToolProfile: "all",
		},
		HTTP: HTTPConfig{
			BindAddress:    "127.0.0.1",
			Port:           8080,
			AllowedOrigins: []string{},
		},
		Logging: LoggingConfig{
			Path:   "/tmp/oba-mcp.log",
			Format: "text",
		},
		Cache: CacheConfig{Path: defaultCachePath()},
	}
	cfg.sources = make(map[field]source, len(configuredFields))
	for _, name := range configuredFields {
		cfg.sources[name] = sourceDefault
	}
	return cfg
}

func defaultCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "/tmp/oba-mcp-cache.db"
	}
	return filepath.Join(dir, "oba-mcp", "cache.db")
}

// RedactedJSON returns the effective configuration without credential values.
func (c Config) RedactedJSON() ([]byte, error) {
	if c.Upstream.APIKey != "" {
		c.Upstream.APIKey = RedactedValue
	}
	if c.HTTP.AuthToken != "" {
		c.HTTP.AuthToken = RedactedValue
	}
	sources := make(map[string]source, len(c.sources))
	for name, value := range c.sources {
		sources[string(name)] = value
	}
	return json.MarshalIndent(struct {
		Configuration Config            `json:"configuration"`
		Sources       map[string]source `json:"sources"`
	}{
		Configuration: c,
		Sources:       sources,
	}, "", "  ")
}

func (c *Config) setSource(name field, value source) {
	if c.sources == nil {
		c.sources = make(map[field]source, len(configuredFields))
	}
	c.sources[name] = value
}

func (c Config) explicitlyConfigured(name field) bool {
	value := c.sources[name]
	return value != "" && value != sourceDefault
}
