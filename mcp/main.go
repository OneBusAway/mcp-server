package main

import (
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"oba-mcp/cachedb"
	"oba-mcp/client"
	"oba-mcp/logger"
	"oba-mcp/tools"

	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/lumberjack.v2"
)

func main() {
	baseURL := envOrDefault("OBA_BASE_URL", "http://localhost:4000")
	apiKey := os.Getenv("OBA_API_KEY")
	if apiKey == "" {
		log.Fatal("OBA_API_KEY must be set")
	}

	// stdout is reserved for MCP JSON-RPC in stdio mode.
	// Logs go to OBA_LOG (default /tmp/oba-mcp.log) with automatic rotation.
	// Format: human-readable by default; set OBA_LOG_JSON=1 for raw JSON.
	logPath := envOrDefault("OBA_LOG", "/tmp/oba-mcp.log")
	jack := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10, // MB before rotation
		MaxBackups: 3,
		MaxAge:     7, // days
		Compress:   true,
	}
	var logDest io.Writer = jack
	if os.Getenv("OBA_LOG_JSON") != "1" {
		logDest = logger.NewPretty(jack)
	}
	appLogger := log.New(logDest, "", 0)

	// Persistent SQLite cache — static data (agencies, routes, stops) survives
	// across sessions so cold starts are instant instead of hitting maglev.
	var db *cachedb.Queries
	cachePath := envOrDefault("OBA_CACHE", defaultCachePath())
	if q, sqlDB, err := cachedb.Open(cachePath); err == nil {
		defer sqlDB.Close()
		db = q
		appLogger.Printf(`{"event":"cache","path":%q}`, cachePath)
	} else {
		appLogger.Printf(`{"event":"cache","error":%q}`, err.Error())
	}

	obaClient := client.New(baseURL, apiKey, appLogger, db)
	toolProfile, err := tools.ParseToolProfile(envOrDefault("OBA_TOOL_PROFILE", string(tools.ToolProfileAll)))
	if err != nil {
		log.Fatal(err)
	}

	s := server.NewMCPServer("OBA Transit Assistant", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
	)
	tools.RegisterProfile(s, obaClient, toolProfile)
	tools.RegisterPrompts(s)

	appLogger.Printf(`{"event":"start","target":%q,"tool_profile":%q}`, baseURL, toolProfile)

	transport := envOrDefault("OBA_TRANSPORT", "stdio")
	if transport == "http" {
		if err := serveHTTP(s, appLogger); err != nil {
			log.Fatal(err)
		}
	} else if transport == "stdio" {
		if err := server.ServeStdio(s); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatalf("unsupported OBA_TRANSPORT %q (expected stdio or http)", transport)
	}

	appLogger.Printf(`{"event":"stop"}`)
}

func serveHTTP(mcpServer *server.MCPServer, appLogger *log.Logger) error {
	token := os.Getenv("OBA_HTTP_AUTH_TOKEN")
	if token == "" {
		return errors.New("OBA_HTTP_AUTH_TOKEN must be set when OBA_TRANSPORT=http")
	}
	origins, err := parseAllowedOrigins(os.Getenv("OBA_ALLOWED_ORIGINS"))
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(envOrDefault("OBA_HTTP_BIND_ADDR", "127.0.0.1"), envOrDefault("OBA_PORT", "8080"))
	httpHandler := newProtectedMCPHTTPHandler(mcpServer, origins, token)
	mux := http.NewServeMux()
	mux.Handle("/mcp", httpHandler)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	appLogger.Printf(`{"event":"http","addr":%q}`, addr)
	return httpServer.ListenAndServe()
}

// newProtectedMCPHTTPHandler builds the complete HTTP transport used in
// production. Keeping it separate from the listener makes the authenticated
// MCP boundary testable without opening a network port.
func newProtectedMCPHTTPHandler(mcpServer *server.MCPServer, origins []string, token string) http.Handler {
	mcpHTTPServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithStreamableHTTPCORS(
			server.WithCORSAllowedOrigins(origins...),
			server.WithCORSAllowedMethods(http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions),
			server.WithCORSAllowedHeaders("Content-Type", "Mcp-Session-Id", "Last-Event-ID", "Authorization"),
			server.WithCORSExposedHeaders("Mcp-Session-Id"),
			server.WithCORSMaxAge(600),
		),
	)
	return protectedHTTPHandler(mcpHTTPServer, origins, token)
}

func defaultCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "/tmp/oba-mcp-cache.db"
	}
	return filepath.Join(dir, "oba-mcp", "cache.db")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
