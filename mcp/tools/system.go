package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
	resp, err := h.client.GetCurrentTime()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.Time == 0 {
		return mcp.NewToolResultText("Could not retrieve server time."), nil
	}

	ms := entry.Time
	readable := entry.ReadableTime
	if readable == "" && ms > 0 {
		readable = time.UnixMilli(int64(ms)).Format(time.RFC3339)
	}

	out, _ := json.MarshalIndent(CurrentTimeResponse{Time: readable}, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Current server time:\n%s", out)), nil
}

func (h *Handler) getMetadata(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Metadata lives at /api/v2/metadata.json (not the standard envelope)
	metadata, err := h.client.GetMetadata()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	output := MetadataResponse{RealtimeFeeds: make(map[string]string, len(metadata.RealtimeFeeds))}
	if metadata.StaticGTFSLastUpdated != nil {
		output.StaticGTFSLastUpdated = metadata.StaticGTFSLastUpdated.Format(time.RFC3339)
	}
	for name, updated := range metadata.RealtimeFeeds {
		output.RealtimeFeeds[name] = updated.Format(time.RFC3339)
	}
	out, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Server metadata:\n%s", out)), nil
}
