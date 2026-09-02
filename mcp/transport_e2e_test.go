package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oba-mcp/client"
	"oba-mcp/internal/obafixture"
	"oba-mcp/tools"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestStdioTransportInitializeAndListTools(t *testing.T) {
	obaClient := client.New("http://example.invalid", "fixture-api-key", nil, nil)
	mcpServer := newApplicationServer(log.New(io.Discard, "", 0), newOperationalMetrics())
	tools.RegisterProfile(mcpServer, obaClient, tools.ToolProfileAll)
	stdioServer := server.NewStdioServer(mcpServer)
	stdioServer.SetErrorLogger(log.New(io.Discard, "", 0))

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- stdioServer.Listen(ctx, stdinReader, stdoutWriter)
	}()
	reader := bufio.NewReader(stdoutReader)

	writeStdioRequest(t, stdinWriter, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+mcp.LATEST_PROTOCOL_VERSION+`","capabilities":{},"clientInfo":{"name":"transport-e2e","version":"1.0.0"}}}`)
	var initialized struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	readStdioResponse(t, reader, &initialized)
	if initialized.Result.ProtocolVersion == "" {
		t.Fatal("stdio initialize response omitted the negotiated protocol version")
	}

	writeStdioRequest(t, stdinWriter, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	var listed struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	readStdioResponse(t, reader, &listed)
	foundStop := false
	for _, tool := range listed.Result.Tools {
		if tool.Name == "get_stop" {
			foundStop = true
			break
		}
	}
	if !foundStop {
		t.Fatal("stdio tools/list did not advertise get_stop")
	}

	cancel()
	stdinWriter.Close()
	select {
	case err := <-result:
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
			t.Fatalf("stdio shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stdio server did not shut down within 5s")
	}
}

func writeStdioRequest(t *testing.T, writer io.Writer, request string) {
	t.Helper()
	if _, err := io.WriteString(writer, request+"\n"); err != nil {
		t.Fatal(err)
	}
}

func readStdioResponse(t *testing.T, reader *bufio.Reader, destination any) {
	t.Helper()
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(line, destination); err != nil {
		t.Fatalf("decode stdio response: %v", err)
	}
}

func TestHTTPTransportInitializeListAndCallTool(t *testing.T) {
	upstream := obafixture.New(map[string]obafixture.Response{
		"/api/where/stop/test_1013.json": {Body: `{
			"code": 200,
			"data": {"entry": {
				"id": "test_1013", "name": "Fixture Stop", "code": "1013",
				"direction": "North", "lat": 27.9488, "lon": -82.4582,
				"routeIds": ["test_10"]
			}}
		}`},
	})
	t.Cleanup(upstream.Close)

	obaClient := client.New(upstream.URL, "fixture-api-key", nil, nil)
	mcpServer := newApplicationServer(log.New(io.Discard, "", 0), newOperationalMetrics())
	tools.RegisterProfile(mcpServer, obaClient, tools.ToolProfileAll)

	mux := http.NewServeMux()
	mux.Handle("/mcp", newProtectedMCPHTTPHandler(mcpServer, []string{"https://app.example"}, "fixture-token"))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	mcpClient, err := mcpclient.NewStreamableHttpClient(httpServer.URL+"/mcp",
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer fixture-token",
			"Origin":        "https://app.example",
		}),
	)
	if err != nil {
		t.Fatalf("create MCP client: %v", err)
	}
	t.Cleanup(func() { _ = mcpClient.Close() })

	ctx := context.Background()
	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("start MCP client: %v", err)
	}
	initialized, err := mcpClient.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "transport-e2e", Version: "1.0.0"},
	}})
	if err != nil {
		t.Fatalf("initialize MCP client: %v", err)
	}
	if initialized.ProtocolVersion == "" {
		t.Fatal("initialize response omitted the negotiated MCP protocol version")
	}

	listed, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if !containsTool(listed.Tools, "get_stop") {
		t.Fatal("get_stop was not advertised by tools/list")
	}

	result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "get_stop",
		Arguments: map[string]any{"stop_id": "test_1013"},
	}})
	if err != nil {
		t.Fatalf("call get_stop: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_stop returned tool error: %#v", result)
	}
	var response tools.SuccessEnvelope[tools.StopResponse]
	if err := json.Unmarshal(result.RawStructuredContent, &response); err != nil {
		t.Fatalf("decode structured get_stop response: %v", err)
	}
	if response.Data.ID != "test_1013" || response.Data.Name != "Fixture Stop" {
		t.Fatalf("structured stop = %#v, want typed fixture stop", response.Data)
	}
	if response.Meta.Cache != string(client.CacheMiss) {
		t.Fatalf("cache state = %q, want miss", response.Meta.Cache)
	}
	if response.Meta.RequestID == "" {
		t.Fatal("structured get_stop response omitted request_id")
	}

	requests := upstream.Requests()
	if len(requests) != 1 || requests[0].Query.Get("key") != "fixture-api-key" {
		t.Fatalf("upstream requests = %#v, want one authenticated fixture request", requests)
	}
}

func TestHTTPTransportRejectsMalformedJSONRPC(t *testing.T) {
	mcpServer := server.NewMCPServer("OBA Transit Assistant", "1.0.0", server.WithToolCapabilities(true))
	handler := newProtectedMCPHTTPHandler(mcpServer, nil, "fixture-token")
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer fixture-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code < http.StatusBadRequest || response.Code >= http.StatusInternalServerError {
		t.Fatalf("malformed JSON-RPC status = %d, want a client error", response.Code)
	}
}

func TestHTTPTransportStartsAndShutsDown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &operationalState{}
	state.setReady(true)
	var lifecycleLogs strings.Builder
	result := make(chan error, 1)
	go func() {
		result <- serveHTTPListener(ctx, httpServer, listener, state, log.New(&lifecycleLogs, "", 0))
	}()

	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		cancel()
		t.Fatalf("request started HTTP transport: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		cancel()
		t.Fatalf("startup status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("graceful shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Streamable HTTP server did not shut down within 5s")
	}
	if state.ready.Load() {
		t.Fatal("readiness remained true after shutdown began")
	}
	if !strings.Contains(lifecycleLogs.String(), `"event":"draining"`) {
		t.Fatalf("shutdown log omitted draining event: %s", lifecycleLogs.String())
	}
}

func TestHTTPTransportRejectsUnauthenticatedToolCalls(t *testing.T) {
	mcpServer := server.NewMCPServer("OBA Transit Assistant", "1.0.0", server.WithToolCapabilities(true))
	mux := http.NewServeMux()
	mux.Handle("/mcp", newProtectedMCPHTTPHandler(mcpServer, nil, "fixture-token"))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	response, err := http.Post(httpServer.URL+"/mcp", "application/json", nil)
	if err != nil {
		t.Fatalf("post unauthenticated request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

// TestHTTPTransportRejectsDisallowedBrowserOrigins covers the browser-origin
// allow-list wired in http_security.go: a browser Origin outside the
// configured list must be rejected at the transport boundary even if the
// caller supplies a valid bearer token.
func TestHTTPTransportRejectsDisallowedBrowserOrigins(t *testing.T) {
	mcpServer := server.NewMCPServer("OBA Transit Assistant", "1.0.0", server.WithToolCapabilities(true))
	mux := http.NewServeMux()
	mux.Handle("/mcp", newProtectedMCPHTTPHandler(mcpServer, []string{"https://app.example"}, "fixture-token"))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/mcp", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer fixture-token")
	request.Header.Set("Origin", "https://evil.example")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("post disallowed-origin request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("disallowed-origin status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

// TestHTTPTransportCancelsInFlightToolCall verifies that context cancellation
// on the MCP client propagates end-to-end: the client returns
// context.Canceled quickly and the upstream request context is torn down so
// the tool handler's HTTP request to OBA is aborted. This is the Phase 3
// cancellation contract at the transport boundary.
//
// The upstream-cancel window is generous (10s) because propagation depends
// on OS/net stack timing when the underlying TCP connection breaks: on
// localhost it is typically sub-millisecond, but a scheduler stall under
// -race can delay the server's ctx.Done fanout. A missed 10s deadline
// signals a real regression in transport cancellation, not test flakiness.
func TestHTTPTransportCancelsInFlightToolCall(t *testing.T) {
	requestArrived := make(chan struct{}, 1)
	upstreamCancelled := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestArrived <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		select {
		case upstreamCancelled <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(upstream.Close)

	obaClient := client.New(upstream.URL, "fixture-api-key", nil, nil)
	mcpServer := server.NewMCPServer("OBA Transit Assistant", "1.0.0", server.WithToolCapabilities(true))
	tools.RegisterProfile(mcpServer, obaClient, tools.ToolProfileAll)

	mux := http.NewServeMux()
	mux.Handle("/mcp", newProtectedMCPHTTPHandler(mcpServer, []string{"https://app.example"}, "fixture-token"))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	mcpClient, err := mcpclient.NewStreamableHttpClient(httpServer.URL+"/mcp",
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer fixture-token",
			"Origin":        "https://app.example",
		}),
	)
	if err != nil {
		t.Fatalf("create MCP client: %v", err)
	}
	t.Cleanup(func() { _ = mcpClient.Close() })

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer setupCancel()
	if err := mcpClient.Start(setupCtx); err != nil {
		t.Fatalf("start MCP client: %v", err)
	}
	if _, err := mcpClient.Initialize(setupCtx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "transport-e2e", Version: "1.0.0"},
	}}); err != nil {
		t.Fatalf("initialize MCP client: %v", err)
	}

	callCtx, cancelCall := context.WithCancel(context.Background())
	callResult := make(chan error, 1)
	go func() {
		_, err := mcpClient.CallTool(callCtx, mcp.CallToolRequest{Params: mcp.CallToolParams{
			Name:      "get_current_time",
			Arguments: map[string]any{},
		}})
		callResult <- err
	}()

	select {
	case <-requestArrived:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not receive the tool call within 5s")
	}

	cancelCall()

	select {
	case err := <-callResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled tool call returned %v, want an error wrapping context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled tool call did not return within 5s of client cancel")
	}

	select {
	case <-upstreamCancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("upstream request context did not cancel within 10s of client cancel: MCP transport did not propagate cancellation to the tool handler")
	}
}

func containsTool(toolList []mcp.Tool, name string) bool {
	for _, tool := range toolList {
		if tool.Name == name {
			return true
		}
	}
	return false
}
