package tools

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerSystemTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_current_time",
			mcp.WithDescription("Get the current server time from the OBA API. Useful for knowing the reference time when interpreting schedules and arrivals."),
			mcp.WithOutputSchema[SuccessEnvelope[CurrentTimeResponse]](),
		),
		h.getCurrentTime,
	)

	s.AddTool(
		mcp.NewTool("get_metadata",
			mcp.WithDescription("Get server metadata: when static GTFS data was last updated and the last refresh time for each real-time feed. Use to check data freshness."),
			mcp.WithOutputSchema[SuccessEnvelope[MetadataResponse]](),
		),
		h.getMetadata,
	)
}

func (h *Handler) getCurrentTime(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := h.client.GetCurrentTime(ctx)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.Time == 0 {
		return toResult(textResult("Could not retrieve server time.")), nil
	}

	ms := entry.Time
	display := entry.ReadableTime
	if display == "" && ms > 0 {
		display = time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339)
	}

	return toResult(withCache(dataResult("Current server time:\n", CurrentTimeResponse{TimeMS: ms, TimeDisplay: display}), string(resp.CacheState))), nil
}

func (h *Handler) getMetadata(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Metadata lives at /api/v2/metadata.json (not the standard envelope)
	metadata, err := h.client.GetMetadata(ctx)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	now := time.Now()
	output := MetadataResponse{RealtimeFeeds: make(map[string]FeedFreshness, len(metadata.RealtimeFeeds))}
	if metadata.StaticGTFSLastUpdated != nil {
		output.StaticGTFSLastUpdatedMS = metadata.StaticGTFSLastUpdated.UnixMilli()
	}
	for name, updated := range metadata.RealtimeFeeds {
		ageSeconds := int64(now.Sub(updated).Seconds())
		output.RealtimeFeeds[name] = FeedFreshness{
			UpdatedAtMS: updated.UnixMilli(),
			AgeSeconds:  max(ageSeconds, 0),
			Status:      freshnessStatus(ageSeconds),
		}
	}
	return toResult(withCache(dataResult("Server metadata:\n", output), string(metadata.CacheState))), nil
}

func freshnessStatus(ageSeconds int64) string {
	switch {
	case ageSeconds <= 60:
		return "fresh"
	case ageSeconds <= 5*60:
		return "delayed"
	default:
		return "stale"
	}
}
