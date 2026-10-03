package tools

import (
	"context"
	"strconv"
	"strings"
	"time"

	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerSystemTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_current_time",
			mcp.WithDescription("Get the current server time from the OBA API. Use only for time-reference questions — not for checking whether the transit system is operational (use get_metadata for that)."),
			mcp.WithOutputSchema[SuccessEnvelope[CurrentTimeResponse]](),
		),
		h.getCurrentTime,
	)

	s.AddTool(
		mcp.NewTool("get_server_config",
			mcp.WithDescription("Identify the OneBusAway server implementation (server_type maglev, java, or unknown) and, outside Maglev, the deployed transit data bundle and its service date range. Use to learn which tools the server supports (get_metadata is Maglev-only) or whether the deployed bundle's service window has ended or is about to."),
			mcp.WithOutputSchema[SuccessEnvelope[ServerConfigResponse]](),
		),
		h.getServerConfig,
	)

	s.AddTool(
		mcp.NewTool("get_metadata",
			mcp.WithDescription("Check whether the transit system is operational: when static GTFS data was last updated and when each real-time feed last refreshed. Use to verify service health or diagnose data staleness — not just for curiosity about timing."),
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

func (h *Handler) getServerConfig(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := h.client.GetServerConfig(ctx)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	config := resp.Data.Entry
	output := ServerConfigResponse{
		ServerType: serverType(config),
		Version:    config.GitProperties.BuildVersion,
		CommitID:   config.GitProperties.CommitID,
	}
	if output.ServerType != "maglev" {
		output.BundleID = config.ID
		output.BundleName = config.Name
		output.ServiceDateFromMS = parseEpochMS(config.ServiceDateFrom)
		output.ServiceDateToMS = parseEpochMS(config.ServiceDateTo)
	}
	return toResult(withCache(dataResult("Server configuration:\n", output), string(resp.CacheState))), nil
}

func serverType(config client.ServerConfig) string {
	if config.ID == "oba-maglev" {
		return "maglev"
	}
	if strings.Contains(config.GitProperties.RemoteOriginURL, "onebusaway-application-modules") {
		return "java"
	}
	return "unknown"
}

func parseEpochMS(value string) int64 {
	ms, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return ms
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
