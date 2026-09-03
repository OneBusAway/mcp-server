package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"oba-mcp/cachedb"
	"oba-mcp/client"
	appconfig "oba-mcp/internal/config"
	"oba-mcp/logger"
	"oba-mcp/tools"

	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/lumberjack.v2"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, checkConfig, printConfig, err := resolveStartupConfig(os.Args[1:], os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}
	validatedConfig := cfg
	validationErr := validatedConfig.Validate()
	if printConfig {
		configToPrint := cfg
		if validationErr == nil {
			configToPrint = validatedConfig
		}
		output, err := configToPrint.RedactedJSON()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(output))
		if validationErr != nil {
			log.Fatal(fmt.Errorf("invalid configuration: %w", validationErr))
		}
		return
	}
	if validationErr != nil {
		log.Fatal(fmt.Errorf("invalid configuration: %w", validationErr))
	}
	cfg = validatedConfig
	if checkConfig {
		fmt.Fprintln(os.Stderr, "configuration is valid")
		return
	}
	if err := prepareLogPath(cfg.Logging.Path); err != nil {
		log.Fatal(err)
	}

	// stdout is reserved for MCP JSON-RPC in stdio mode.
	// Logs go to OBA_LOG (default /tmp/oba-mcp.log) with automatic rotation.
	// Format: human-readable by default; set OBA_LOG_FORMAT=json for raw JSON.
	jack := &lumberjack.Logger{
		Filename:   cfg.Logging.Path,
		MaxSize:    10, // MB before rotation
		MaxBackups: 3,
		MaxAge:     7, // days
		Compress:   true,
	}
	defer func() {
		if err := jack.Close(); err != nil {
			log.Printf("close log writer: %v", err)
		}
	}()
	var logDest io.Writer = jack
	if cfg.Logging.Format != "json" {
		logDest = logger.NewPretty(jack)
	}
	appLogger := log.New(logDest, "", 0)

	// Persistent SQLite cache — static data (agencies, routes, stops) survives
	// across sessions so cold starts are instant instead of hitting maglev.
	db, sqlDB, err := openPersistentCache(cfg.Cache.Path)
	if err != nil {
		log.Fatal(err)
	}
	if sqlDB == nil {
		appLogger.Printf(`{"event":"cache","state":"memory-only"}`)
	} else {
		defer sqlDB.Close()
		appLogger.Printf(`{"event":"cache","path":%q}`, cfg.Cache.Path)
	}

	metrics := newOperationalMetrics()
	obaClient := client.New(cfg.Upstream.BaseURL, cfg.Upstream.APIKey, appLogger, db)
	obaClient.SetObserver(metrics)
	toolProfile, err := tools.ParseToolProfile(cfg.MCP.ToolProfile)
	if err != nil {
		log.Fatal(err)
	}

	s := newApplicationServer(appLogger, metrics)
	tools.RegisterProfile(s, obaClient, toolProfile)
	tools.RegisterPrompts(s)

	appLogger.Printf(`{"event":"start","target":%q,"tool_profile":%q}`, cfg.Upstream.BaseURL, toolProfile)

	if cfg.MCP.Transport == appconfig.TransportStreamableHTTP {
		if err := serveHTTP(ctx, s, appLogger, cfg.HTTP, metrics, os.Stderr); err != nil {
			log.Fatal(err)
		}
	} else {
		announceReady(appLogger, os.Stderr, string(appconfig.TransportStdio), "")
		if err := server.ServeStdio(s); err != nil {
			log.Fatal(err)
		}
	}

	appLogger.Printf(`{"event":"stop"}`)
}

func newApplicationServer(appLogger *log.Logger, metrics *operationalMetrics) *server.MCPServer {
	return server.NewMCPServer("OBA Transit Assistant", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithToolHandlerMiddleware(toolObservabilityMiddleware(appLogger, metrics)),
		server.WithToolHandlerMiddleware(safeRecoveryMiddleware(appLogger)),
	)
}

func prepareLogPath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create log directory for %q: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log path %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close log path %q: %w", path, err)
	}
	return nil
}

func openPersistentCache(path string) (*cachedb.Queries, *sql.DB, error) {
	if path == "" {
		return nil, nil, nil
	}
	queries, database, err := cachedb.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open persistent cache %q: %w", path, err)
	}
	return queries, database, nil
}

func serveHTTP(ctx context.Context, mcpServer *server.MCPServer, appLogger *log.Logger, cfg appconfig.HTTPConfig, metrics *operationalMetrics, statusWriter io.Writer) error {
	addr := net.JoinHostPort(cfg.BindAddress, fmt.Sprintf("%d", cfg.Port))
	httpHandler := newProtectedMCPHTTPHandler(mcpServer, cfg.AllowedOrigins, cfg.AuthToken)
	state := &operationalState{}
	mux := newHTTPMux(httpHandler, state, metrics, cfg.AuthToken)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	endpoint := "http://" + listener.Addr().String() + "/mcp"
	state.setReady(true)
	defer state.setReady(false)
	announceReady(appLogger, statusWriter, string(appconfig.TransportStreamableHTTP), endpoint)
	return serveHTTPListener(ctx, httpServer, listener, state, appLogger)
}

func newHTTPMux(mcpHandler http.Handler, state *operationalState, metrics *operationalMetrics, authToken string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/healthz", state.healthHandler)
	mux.HandleFunc("/readyz", state.readinessHandler)
	mux.Handle("/metrics", protectedHTTPHandler(http.HandlerFunc(metrics.handler), nil, authToken))
	return mux
}

func serveHTTPListener(ctx context.Context, httpServer *http.Server, listener net.Listener, state *operationalState, appLogger *log.Logger) error {
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- httpServer.Serve(listener)
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		state.setReady(false)
		appLogger.Printf(`{"event":"draining","timeout_ms":5000}`)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
			<-serveResult
			return fmt.Errorf("shut down Streamable HTTP server: %w", err)
		}
		err := <-serveResult
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// newProtectedMCPHTTPHandler builds the complete HTTP transport used in
// production. Keeping it separate from the listener makes the authenticated
// MCP boundary testable without opening a network port.
func newProtectedMCPHTTPHandler(mcpServer *server.MCPServer, origins []string, token string) http.Handler {
	mcpHTTPServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithStreamableHTTPCORS(
			server.WithCORSAllowedOrigins(origins...),
			server.WithCORSAllowedMethods(http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions),
			server.WithCORSAllowedHeaders("Content-Type", "Mcp-Session-Id", "Last-Event-ID", "Authorization", "X-Request-ID"),
			server.WithCORSExposedHeaders("Mcp-Session-Id", "X-Request-ID"),
			server.WithCORSMaxAge(600),
		),
	)
	return protectedHTTPHandler(mcpHTTPServer, origins, token)
}

// resolveStartupConfig loads source files and environment values without
// semantic validation so --print-config can diagnose invalid combinations.
func resolveStartupConfig(args []string, lookupEnv func(string) (string, bool)) (appconfig.Config, bool, bool, error) {
	flags := flag.NewFlagSet("oba-mcp", flag.ContinueOnError)
	configFile := flags.String("config", "", "path to JSON configuration file")
	envFile := flags.String("env-file", "", "path to dotenv configuration file")
	checkConfig := flags.Bool("check-config", false, "validate configuration and exit")
	printConfig := flags.Bool("print-config", false, "print redacted effective configuration and exit")
	if err := flags.Parse(args); err != nil {
		return appconfig.Config{}, false, false, err
	}
	if flags.NArg() != 0 {
		return appconfig.Config{}, false, false, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if *configFile == "" {
		*configFile, _ = lookupEnv("OBA_CONFIG_FILE")
	}
	if *envFile == "" {
		*envFile, _ = lookupEnv("OBA_ENV_FILE")
	}
	cfg, err := appconfig.Resolve(appconfig.LoadOptions{
		ConfigFile: *configFile,
		EnvFile:    *envFile,
		LookupEnv:  lookupEnv,
	})
	if err != nil {
		return appconfig.Config{}, false, false, err
	}
	return cfg, *checkConfig, *printConfig, nil
}
