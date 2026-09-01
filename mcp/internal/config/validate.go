package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Validate checks and normalizes the resolved startup configuration.
func (c *Config) Validate() error {
	if err := validateBaseURL(c.Upstream.BaseURL); err != nil {
		return err
	}
	if c.Upstream.APIKey == "" {
		return errors.New("upstream.api-key is required (set OBA_API_KEY or configure it in a local secret file)")
	}
	if c.MCP.Transport == "http" {
		// Compatibility alias for deployments created before the transport used
		// the official MCP name. Diagnostics always report the canonical value.
		c.MCP.Transport = TransportStreamableHTTP
	}
	if c.MCP.Transport != TransportStdio && c.MCP.Transport != TransportStreamableHTTP {
		return fmt.Errorf("mcp.transport must be stdio or streamable-http, got %q", c.MCP.Transport)
	}
	if c.MCP.ToolProfile != "all" && c.MCP.ToolProfile != "rider" {
		return fmt.Errorf("mcp.tool-profile must be all or rider, got %q", c.MCP.ToolProfile)
	}
	if c.MCP.Transport == TransportStdio {
		if name := c.explicitHTTPField(); name != "" {
			return fmt.Errorf("%s is HTTP-only and must not be configured when mcp.transport is stdio", name)
		}
	} else {
		if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
			return fmt.Errorf("http.port must be between 1 and 65535, got %d", c.HTTP.Port)
		}
		if !validBindAddress(c.HTTP.BindAddress) {
			return errors.New("http.bind-address must be an IP address or hostname")
		}
		origins, err := validateOrigins(c.HTTP.AllowedOrigins)
		if err != nil {
			return err
		}
		c.HTTP.AllowedOrigins = origins
		if c.HTTP.AuthToken == "" {
			return errors.New("http.auth-token is required when mcp.transport is streamable-http (set OBA_HTTP_AUTH_TOKEN)")
		}
	}
	if c.Logging.Format != "text" && c.Logging.Format != "json" {
		return fmt.Errorf("logging.format must be text or json, got %q", c.Logging.Format)
	}
	if strings.TrimSpace(c.Logging.Path) == "" {
		return errors.New("logging.path must not be empty")
	}
	return nil
}

func (c Config) explicitHTTPField() field {
	for _, name := range []field{fieldBindAddress, fieldPort, fieldAuthToken, fieldAllowedOrigins} {
		if c.explicitlyConfigured(name) {
			return name
		}
	}
	return ""
}

func validBindAddress(value string) bool {
	if value == "localhost" || net.ParseIP(value) != nil {
		return true
	}
	if value == "" || len(value) > 253 || strings.HasSuffix(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if character != '-' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
				return false
			}
		}
	}
	return true
}

func validateBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("upstream.base-url must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return errors.New("upstream.base-url must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("upstream.base-url must not contain a query or fragment")
	}
	return nil
}

// ParseAllowedOrigins parses the comma-separated environment representation.
func ParseAllowedOrigins(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	origins := strings.Split(raw, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
		if origins[i] == "" {
			return nil, errors.New("OBA_ALLOWED_ORIGINS contains an empty origin")
		}
	}
	validated, err := validateOrigins(origins)
	if err != nil {
		return nil, fmt.Errorf("OBA_ALLOWED_ORIGINS: %w", err)
	}
	return validated, nil
}

func validateOrigins(origins []string) ([]string, error) {
	seen := make(map[string]struct{}, len(origins))
	validated := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin == "*" {
			return nil, errors.New("http.allowed-origins must not contain a wildcard")
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.String() != origin {
			return nil, fmt.Errorf("invalid HTTP origin %q", origin)
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		validated = append(validated, origin)
	}
	return validated, nil
}
