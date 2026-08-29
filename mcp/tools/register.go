// Package tools registers all OBA MCP tools onto an MCP server.
package tools

import (
	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/server"
)

// Handler holds shared dependencies for all tool handlers.
type Handler struct {
	client *client.OBAClient
}

// RegisterAll adds every OBA tool to s.
func RegisterAll(s *server.MCPServer, c *client.OBAClient) {
	h := &Handler{client: c}
	h.registerSystemTools(s)
	h.registerAgencyTools(s)
	h.registerStopTools(s)
	h.registerArrivalTools(s)
	h.registerRouteTools(s)
	h.registerTripTools(s)
	h.registerOverviewTools(s)
}
