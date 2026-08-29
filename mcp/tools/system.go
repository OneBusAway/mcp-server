package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"oba-mcp/client"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerSystemTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_current_time",
			mcp.WithDescription("Get the current server time from the OBA API. Useful for knowing the reference time when interpreting schedules and arrivals."),
		),
		h.getCurrentTime,
	)

	s.AddTool(
		mcp.NewTool("get_metadata",
			mcp.WithDescription("Get server metadata: when static GTFS data was last updated and the last refresh time for each real-time feed. Use to check data freshness."),
		),
		h.getMetadata,
	)
}

func (h *Handler) getCurrentTime(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := h.client.Get("/api/where/current-time.json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText("Could not retrieve server time."), nil
	}

	ms := client.FloatVal(entry["time"])
	readable := client.StrVal(entry["readableTime"])
	if readable == "" && ms > 0 {
		readable = time.UnixMilli(int64(ms)).Format(time.RFC3339)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Current server time: %s", readable)), nil
}

func (h *Handler) getMetadata(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Metadata lives at /api/v2/metadata.json (not the standard envelope)
	raw, err := h.client.Get("/api/v2/metadata.json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	out, _ := json.MarshalIndent(raw, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Server metadata:\n%s", out)), nil
}
