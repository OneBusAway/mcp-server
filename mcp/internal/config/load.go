package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const maxConfigFileBytes = 1 << 20 // 1 MiB

// LoadOptions selects optional configuration files and the process environment.
type LoadOptions struct {
	ConfigFile string
	EnvFile    string
	LookupEnv  func(string) (string, bool)
}

// Resolve overlays defaults, JSON, dotenv, and process environment in that
// order. It performs source parsing but leaves semantic validation to callers.
func Resolve(options LoadOptions) (Config, error) {
	cfg := Defaults()
	if options.ConfigFile != "" {
		if err := loadJSON(options.ConfigFile, &cfg); err != nil {
			return Config{}, err
		}
	}

	if options.EnvFile != "" {
		values, err := loadDotenv(options.EnvFile)
		if err != nil {
			return Config{}, err
		}
		if err := applyEnvironment(&cfg, mapLookup(values), sourceDotenv); err != nil {
			return Config{}, fmt.Errorf("invalid dotenv configuration: %w", err)
		}
	}

	lookup := options.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if err := applyEnvironment(&cfg, lookup, sourceEnvironment); err != nil {
		return Config{}, fmt.Errorf("invalid process environment: %w", err)
	}
	return cfg, nil
}

// Load resolves then validates configuration for normal server startup.
func Load(options LoadOptions) (Config, error) {
	cfg, err := Resolve(options)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

func loadJSON(path string, cfg *Config) error {
	data, err := readBoundedFile(path)
	if err != nil {
		return fmt.Errorf("load config file %q: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	if err := rejectJSONNulls(data); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	markJSONSources(cfg, data)
	return nil
}

func markJSONSources(cfg *Config, data []byte) {
	var document map[string]any
	if json.Unmarshal(data, &document) != nil {
		return
	}
	paths := map[string]map[string]field{
		"upstream": {
			"base-url": fieldBaseURL,
			"api-key":  fieldAPIKey,
		},
		"mcp": {
			"transport":    fieldTransport,
			"tool-profile": fieldToolProfile,
		},
		"http": {
			"bind-address":    fieldBindAddress,
			"port":            fieldPort,
			"auth-token":      fieldAuthToken,
			"allowed-origins": fieldAllowedOrigins,
		},
		"logging": {
			"path":   fieldLogPath,
			"format": fieldLogFormat,
		},
		"cache": {
			"path": fieldCachePath,
		},
	}
	for sectionName, fields := range paths {
		section, ok := document[sectionName].(map[string]any)
		if !ok {
			continue
		}
		for property, name := range fields {
			if _, exists := section[property]; exists {
				cfg.setSource(name, sourceJSON)
			}
		}
	}
}

func rejectJSONNulls(data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return rejectNullValue(value, "configuration")
}

func rejectNullValue(value any, path string) error {
	if value == nil {
		return fmt.Errorf("%s must not be null", path)
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if err := rejectNullValue(child, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := rejectNullValue(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadDotenv(path string) (map[string]string, error) {
	data, err := readBoundedFile(path)
	if err != nil {
		return nil, fmt.Errorf("load env file %q: %w", path, err)
	}
	values, err := godotenv.Unmarshal(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse env file %q: invalid dotenv syntax", path)
	}
	return values, nil
}

func readBoundedFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("must be a regular file")
	}
	if info.Size() > maxConfigFileBytes {
		return nil, fmt.Errorf("file exceeds %d-byte limit", maxConfigFileBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxConfigFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxConfigFileBytes {
		return nil, fmt.Errorf("file exceeds %d-byte limit", maxConfigFileBytes)
	}
	return data, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON documents are not allowed")
		}
		return err
	}
	return nil
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func applyEnvironment(cfg *Config, lookup func(string) (string, bool), valueSource source) error {
	applyString(cfg, lookup, "OBA_BASE_URL", fieldBaseURL, &cfg.Upstream.BaseURL, valueSource)
	applyString(cfg, lookup, "OBA_API_KEY", fieldAPIKey, &cfg.Upstream.APIKey, valueSource)
	applyString(cfg, lookup, "OBA_TRANSPORT", fieldTransport, &cfg.MCP.Transport, valueSource)
	applyString(cfg, lookup, "OBA_TOOL_PROFILE", fieldToolProfile, &cfg.MCP.ToolProfile, valueSource)
	applyString(cfg, lookup, "OBA_HTTP_BIND_ADDR", fieldBindAddress, &cfg.HTTP.BindAddress, valueSource)
	applyString(cfg, lookup, "OBA_HTTP_AUTH_TOKEN", fieldAuthToken, &cfg.HTTP.AuthToken, valueSource)
	applyString(cfg, lookup, "OBA_LOG", fieldLogPath, &cfg.Logging.Path, valueSource)
	applyString(cfg, lookup, "OBA_CACHE", fieldCachePath, &cfg.Cache.Path, valueSource)

	if value, ok := lookup("OBA_PORT"); ok {
		port, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return errors.New("OBA_PORT must be an integer")
		}
		cfg.HTTP.Port = port
		cfg.setSource(fieldPort, valueSource)
	}
	if value, ok := lookup("OBA_ALLOWED_ORIGINS"); ok {
		origins, err := ParseAllowedOrigins(value)
		if err != nil {
			return err
		}
		cfg.HTTP.AllowedOrigins = origins
		cfg.setSource(fieldAllowedOrigins, valueSource)
	}
	logFormat, hasLogFormat := lookup("OBA_LOG_FORMAT")
	logJSON, hasLogJSON := lookup("OBA_LOG_JSON")
	var legacyFormat string
	if hasLogJSON {
		switch strings.ToLower(strings.TrimSpace(logJSON)) {
		case "1", "true":
			legacyFormat = "json"
		case "0", "false":
			legacyFormat = "text"
		default:
			return errors.New("OBA_LOG_JSON must be one of 0, 1, false, or true")
		}
	}
	if hasLogFormat {
		logFormat = strings.ToLower(strings.TrimSpace(logFormat))
		if hasLogJSON && logFormat != legacyFormat {
			return errors.New("OBA_LOG_FORMAT conflicts with OBA_LOG_JSON")
		}
		cfg.Logging.Format = logFormat
		cfg.setSource(fieldLogFormat, valueSource)
	} else if hasLogJSON {
		cfg.Logging.Format = legacyFormat
		cfg.setSource(fieldLogFormat, valueSource)
	}
	return nil
}

func applyString(cfg *Config, lookup func(string) (string, bool), key string, name field, destination *string, valueSource source) {
	if value, ok := lookup(key); ok {
		*destination = value
		cfg.setSource(name, valueSource)
	}
}
