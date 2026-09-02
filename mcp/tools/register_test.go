package tools

import (
	"strings"
	"testing"

	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterProfile(t *testing.T) {
	client := client.New("http://example.invalid", "test-key", nil, nil)

	t.Run("rider", func(t *testing.T) {
		server := server.NewMCPServer("test", "test")
		RegisterProfile(server, client, ToolProfileRider)
		tools := server.ListTools()
		if len(tools) != 16 {
			t.Fatalf("tool count = %d, want 16", len(tools))
		}
		if _, ok := tools["get_shape"]; ok {
			t.Fatal("rider profile includes get_shape")
		}
		if _, ok := tools["get_arrivals_for_stop"]; !ok {
			t.Fatal("rider profile is missing get_arrivals_for_stop")
		}
		if _, ok := tools["get_stops_for_route"]; !ok {
			t.Fatal("rider profile is missing route map data")
		}
		if _, ok := tools["get_trip_details"]; !ok {
			t.Fatal("rider profile is missing live vehicle data")
		}
	})

	t.Run("all", func(t *testing.T) {
		server := server.NewMCPServer("test", "test")
		RegisterProfile(server, client, ToolProfileAll)
		if got := len(server.ListTools()); got != 29 {
			t.Fatalf("tool count = %d, want 29", got)
		}
	})
}

func TestParseToolProfile(t *testing.T) {
	if _, err := ParseToolProfile("unsupported"); err == nil {
		t.Fatal("unsupported profile was accepted")
	}
}

func TestToolDescriptionsPreserveSelectionBoundaries(t *testing.T) {
	server := server.NewMCPServer("test", "test")
	RegisterAll(server, client.New("http://example.invalid", "test-key", nil, nil))
	registered := server.ListTools()

	for toolName, want := range map[string]string{
		"search_stops":             "landmark but gives no numeric coordinates",
		"find_stops_near_location": "Never geocode a landmark",
		"get_trip_details":         "never substitute a stop_id or route_id",
	} {
		tool, ok := registered[toolName]
		if !ok {
			t.Fatalf("tool %q is not registered", toolName)
		}
		if !strings.Contains(tool.Tool.Description, want) {
			t.Fatalf("tool %q description = %q, want %q", toolName, tool.Tool.Description, want)
		}
	}
	for _, want := range []string{
		"get_stop_overview once",
		"never guess a trip_id from a stop_id or route_id",
		"refuse it without calling an unrelated tool",
		"do not retry the request through an unrelated tool",
		"Never pass a stop_id as a trip_id",
	} {
		if !strings.Contains(SystemPrompt(), want) {
			t.Fatalf("system prompt is missing workflow guidance %q", want)
		}
	}
}
