package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerOverviewTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_stop_overview",
			mcp.WithDescription("Get a quick summary of a stop: name, location, routes serving it, and the next 5 arrivals in the next 30 minutes. Use this for 'what's at stop X?' or 'what's coming soon?' — replaces the common pattern of get_stop + get_arrivals_for_stop."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
		),
		h.getStopOverview,
	)
}

type stopOverview struct {
	StopID   string           `json:"stop_id"`
	StopName string           `json:"stop_name,omitempty"`
	Lat      float64          `json:"lat,omitempty"`
	Lon      float64          `json:"lon,omitempty"`
	Routes   []string         `json:"routes_serving,omitempty"`
	Next     []arrivalSummary `json:"next_arrivals,omitempty"`
}

func (h *Handler) getStopOverview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := req.RequireString("stop_id")
	if err != nil || stopID == "" {
		return mcp.NewToolResultError("stop_id is required"), nil
	}

	loc := h.client.TimezoneFor(client.AgencyIDFromEntityID(stopID))

	params := url.Values{
		"minutesBefore": {"0"},
		"minutesAfter":  {"30"},
	}
	resp, err := h.client.Get("/api/where/arrivals-and-departures-for-stop/"+stopID+".json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No data found for stop %s.", stopID)), nil
	}

	overview := stopOverview{StopID: stopID}

	if refs, ok := data["references"].(map[string]any); ok {
		for _, s := range client.AsSlice(refs["stops"]) {
			stop, ok := s.(map[string]any)
			if !ok {
				continue
			}
			if client.StrVal(stop["id"]) == stopID {
				overview.StopName = client.StrVal(stop["name"])
				overview.Lat = client.FloatVal(stop["lat"])
				overview.Lon = client.FloatVal(stop["lon"])
				break
			}
		}
	}

	seen := map[string]bool{}
	for _, item := range client.AsSlice(entry["arrivalsAndDepartures"]) {
		a, ok := item.(map[string]any)
		if !ok {
			continue
		}
		routeID := client.StrVal(a["routeId"])
		if routeID != "" && !seen[routeID] {
			seen[routeID] = true
			overview.Routes = append(overview.Routes, routeID)
		}
		if len(overview.Next) >= 5 {
			continue
		}
		ar := arrivalSummary{
			TripID:      client.StrVal(a["tripId"]),
			ServiceDate: int64(client.FloatVal(a["serviceDate"])),
			RouteID:     routeID,
			RouteName:   client.StrVal(a["routeShortName"]),
			Headsign:    client.StrVal(a["tripHeadsign"]),
			Status:      client.StrVal(a["status"]),
			VehicleID:   client.StrVal(a["vehicleId"]),
			Predicted:   a["predicted"] == true,
		}
		if value, ok := a["numberOfStopsAway"]; ok {
			stopsAway := int(client.FloatVal(value))
			ar.NumberOfStopsAway = &stopsAway
		}
		if scheduled, ok := a["scheduledArrivalTime"].(float64); ok && scheduled > 0 {
			ar.ScheduledArrival = client.FormatRelativeTime(scheduled, loc)
		}
		if schedDep, ok := a["scheduledDepartureTime"].(float64); ok && schedDep > 0 {
			ar.ScheduledDeparture = client.FormatRelativeTime(schedDep, loc)
		}
		if ar.Predicted {
			if predicted, ok := a["predictedArrivalTime"].(float64); ok && predicted > 0 {
				ar.PredictedArrival = client.FormatRelativeTime(predicted, loc)
			}
			if predDep, ok := a["predictedDepartureTime"].(float64); ok && predDep > 0 {
				ar.PredictedDeparture = client.FormatRelativeTime(predDep, loc)
			}
		}
		overview.Next = append(overview.Next, ar)
	}

	out, _ := json.MarshalIndent(overview, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Overview for stop %s:\n%s", stopID, out)), nil
}
