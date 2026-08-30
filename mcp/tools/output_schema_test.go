package tools

import (
	"testing"

	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/server"
)

func TestEveryToolPublishesAStructuredOutputSchema(t *testing.T) {
	server := server.NewMCPServer("test", "test")
	RegisterAll(server, client.New("http://example.invalid", "test-key", nil, nil))

	tools := server.ListTools()
	if len(tools) != 29 {
		t.Fatalf("tool count = %d, want 29", len(tools))
	}
	for name, registered := range tools {
		schema := registered.Tool.OutputSchema
		if schema.Type != "object" {
			t.Errorf("%s output schema type = %q, want object", name, schema.Type)
		}
		if _, ok := schema.Properties["data"]; !ok {
			t.Errorf("%s output schema does not define data", name)
		}
		if _, ok := schema.Properties["meta"]; !ok {
			t.Errorf("%s output schema does not define meta", name)
		}
	}
}
