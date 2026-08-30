// Package tools registers all OBA MCP tools onto an MCP server.
package tools

import (
	"fmt"
	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/server"
)

// ToolProfile controls which tools are advertised to an MCP client.
type ToolProfile string

const (
	ToolProfileRider ToolProfile = "rider"
	ToolProfileAll   ToolProfile = "all"
)

var advancedToolNames = []string{
	"get_arrival_and_departure_for_stop",
	"get_block",
	"get_metadata",
	"get_route_ids_for_agency",
	"get_schedule_for_route",
	"get_shape",
	"get_stop_ids_for_agency",
	"get_stop_schedule",
	"get_stops_for_agency",
	"get_stops_for_route",
	"get_trip_details",
	"get_trip_for_vehicle",
	"get_trips_for_location",
	"get_trips_for_route",
	"get_vehicles_for_agency",
}

// ParseToolProfile validates the OBA_TOOL_PROFILE value.
func ParseToolProfile(value string) (ToolProfile, error) {
	switch ToolProfile(value) {
	case ToolProfileRider, ToolProfileAll:
		return ToolProfile(value), nil
	default:
		return "", fmt.Errorf("unsupported tool profile %q (expected rider or all)", value)
	}
}

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

// RegisterProfile registers the selected tool profile. The rider profile is
// intentionally limited to common passenger workflows; all exposes every tool.
func RegisterProfile(s *server.MCPServer, c *client.OBAClient, profile ToolProfile) {
	RegisterAll(s, c)
	if profile == ToolProfileRider {
		s.DeleteTools(advancedToolNames...)
	}
}
