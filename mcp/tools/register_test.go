package tools

import (
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
		if len(tools) != 14 {
			t.Fatalf("tool count = %d, want 14", len(tools))
		}
		if _, ok := tools["get_shape"]; ok {
			t.Fatal("rider profile includes get_shape")
		}
		if _, ok := tools["get_arrivals_for_stop"]; !ok {
			t.Fatal("rider profile is missing get_arrivals_for_stop")
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
