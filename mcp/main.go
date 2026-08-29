package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"oba-mcp/cachedb"
	"oba-mcp/client"
	"oba-mcp/logger"
	"oba-mcp/tools"

	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/lumberjack.v2"
)

func main() {
	baseURL := envOrDefault("OBA_BASE_URL", "http://localhost:4000")
	apiKey := envOrDefault("OBA_API_KEY", "test")

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

	s := server.NewMCPServer("OBA Transit Assistant", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
	)
	tools.RegisterAll(s, obaClient)
	tools.RegisterPrompts(s)

	appLogger.Printf(`{"event":"start","target":%q}`, baseURL)

	transport := envOrDefault("OBA_TRANSPORT", "stdio")
	if transport == "http" {
		port := envOrDefault("OBA_PORT", "8080")
		addr := fmt.Sprintf(":%s", port)
		appLogger.Printf(`{"event":"http","addr":%q}`, addr)
		httpServer := server.NewStreamableHTTPServer(s,
			server.WithStreamableHTTPCORS(
				server.WithCORSAllowedOrigins("*"),
			),
			server.WithDisableLocalhostProtection(true),
		)
		if err := httpServer.Start(addr); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := server.ServeStdio(s); err != nil {
			log.Fatal(err)
		}
	}

	appLogger.Printf(`{"event":"stop"}`)
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
