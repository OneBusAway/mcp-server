package tools

import (
	"context"
	"fmt"
	"net/url"
	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerOverviewTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_stop_overview",
			mcp.WithDescription("Get a quick summary of a stop: name, location, routes serving it, and the next 5 arrivals with headsigns. Prefer this over calling get_stop then get_arrivals_for_stop separately — it answers both in one call. Use for 'what buses stop here?', 'what's coming soon?', or 'what direction is the next bus?'"),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
			mcp.WithOutputSchema[SuccessEnvelope[StopOverviewResponse]](),
		),
		h.getStopOverview,
	)
}

func (h *Handler) getStopOverview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := entityIDArgument(req, "stop_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	loc := h.client.TimezoneFor(ctx, client.AgencyIDFromEntityID(stopID))

	params := url.Values{
		"minutesBefore": {"0"},
		"minutesAfter":  {"60"},
	}
	resp, err := h.client.ArrivalsForStop(ctx, stopID, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.StopID == "" {
		return toResult(textResult(fmt.Sprintf("No data found for stop %s.", stopID))), nil
	}

	overview := StopOverviewResponse{StopID: stopID}

	for _, stop := range resp.Data.References.Stops {
		if stop.ID == stopID {
			overview.StopName = stop.Name
			overview.Lat = stop.Lat
			overview.Lon = stop.Lon
			break
		}
	}

	seen := map[string]bool{}
	for _, arrival := range entry.ArrivalsAndDepartures {
		routeID := arrival.RouteID
		if routeID != "" && !seen[routeID] {
			seen[routeID] = true
			overview.Routes = append(overview.Routes, routeID)
		}
		if len(overview.Next) >= 5 {
			continue
		}
		overview.Next = append(overview.Next, arrivalResponse(arrival, resp.Data.References.Trips, loc))
	}

	return toResult(withCache(dataResult(fmt.Sprintf("Overview for stop %s:\n", stopID), overview), string(resp.CacheState))), nil
}
