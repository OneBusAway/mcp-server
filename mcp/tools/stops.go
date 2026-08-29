package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"oba-mcp/client"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerStopTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_stop_ids_for_agency",
			mcp.WithDescription("Get a flat list of all stop IDs for an agency. Useful when you need to enumerate stop IDs programmatically. Use get_stops_for_agency if you also need names and locations."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
		),
		h.getStopIDsForAgency,
	)

	s.AddTool(
		mcp.NewTool("get_stop",
			mcp.WithDescription("Look up a stop by ID and return its name, location, direction, and routes. Use this when you already have a stop_id — faster than searching. IDs look like 'unitrans_22274'."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
		),
		h.getStop,
	)

	s.AddTool(
		mcp.NewTool("search_stops",
			mcp.WithDescription("Search stops by name or street keyword. Use when you don't have a stop_id yet. Returns matching stops with their IDs."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Stop name, street, or code to search for")),
			mcp.WithNumber("max_count", mcp.Description("Max results to return (default: 5)")),
		),
		h.searchStops,
	)

	s.AddTool(
		mcp.NewTool("find_stops_near_location",
			mcp.WithDescription("Find transit stops near GPS coordinates. Returns nearby stop IDs, names, directions, and routes."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters (default: 500, max: 5000)")),
			mcp.WithNumber("lat_span", mcp.Description("Bounding box latitude span (alternative to radius)")),
			mcp.WithNumber("lon_span", mcp.Description("Bounding box longitude span (alternative to radius)")),
			mcp.WithNumber("max_count", mcp.Description("Max stops to return (default: 10, max: 20). Pass the number the user asked for.")),
		),
		h.findStopsNearLocation,
	)

	s.AddTool(
		mcp.NewTool("get_stops_for_agency",
			mcp.WithDescription("List stops operated by an agency. Returns up to 50 by default — use find_stops_near_location to filter by area."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
		),
		h.getStopsForAgency,
	)

	s.AddTool(
		mcp.NewTool("get_stop_schedule",
			mcp.WithDescription("Get the full day schedule for a stop grouped by route and direction, with trip_id per departure. Note: some trips may be missing if the backend omits a direction grouping — use get_arrivals_for_stop with a time= window for guaranteed completeness at a specific hour."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
			mcp.WithString("date", mcp.Description("Date in YYYY-MM-DD format (defaults to today)")),
		),
		h.getStopSchedule,
	)
}

type stopSummary struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Code      string   `json:"code"`
	Direction string   `json:"direction,omitempty"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	RouteIDs  []string `json:"route_ids,omitempty"`
}

func stopFromMap(m map[string]any) stopSummary {
	s := stopSummary{
		ID:        client.StrVal(m["id"]),
		Name:      client.StrVal(m["name"]),
		Code:      client.StrVal(m["code"]),
		Direction: client.StrVal(m["direction"]),
		Lat:       client.FloatVal(m["lat"]),
		Lon:       client.FloatVal(m["lon"]),
	}
	for _, id := range client.AsSlice(m["routeIds"]) {
		s.RouteIDs = append(s.RouteIDs, fmt.Sprintf("%v", id))
	}
	return s
}

func (h *Handler) getStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := req.RequireString("stop_id")
	if err != nil || stopID == "" {
		return mcp.NewToolResultError("stop_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/stop/"+stopID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No stop found with ID %q.", stopID)), nil
	}

	out, _ := json.MarshalIndent(stopFromMap(entry), "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Stop %s:\n%s", stopID, out)), nil
}

func (h *Handler) searchStops(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	params := url.Values{
		"input":    {query},
		"maxCount": {fmt.Sprintf("%.0f", req.GetFloat("max_count", 5))},
	}

	resp, err := h.client.Get("/api/where/search/stop.json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	if len(list) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No stops found matching %q.", query)), nil
	}

	results := make([]stopSummary, 0, len(list))
	for _, item := range list {
		stop, ok := item.(map[string]any)
		if !ok {
			continue
		}
		results = append(results, stopFromMap(stop))
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Found %d stops matching %q:\n%s", len(results), query, out)), nil
}

func (h *Handler) findStopsNearLocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lat, err := req.RequireFloat("lat")
	if err != nil {
		return mcp.NewToolResultError("lat is required"), nil
	}
	lon, err := req.RequireFloat("lon")
	if err != nil {
		return mcp.NewToolResultError("lon is required"), nil
	}
	radius := req.GetFloat("radius", 500)

	params := url.Values{
		"lat":    {fmt.Sprintf("%f", lat)},
		"lon":    {fmt.Sprintf("%f", lon)},
		"radius": {fmt.Sprintf("%.0f", radius)},
	}
	if ls := req.GetFloat("lat_span", 0); ls > 0 {
		params.Set("latSpan", fmt.Sprintf("%f", ls))
	}
	if ls := req.GetFloat("lon_span", 0); ls > 0 {
		params.Set("lonSpan", fmt.Sprintf("%f", ls))
	}

	resp, err := h.client.Get("/api/where/stops-for-location.json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	if len(list) == 0 {
		return mcp.NewToolResultText("No stops found within the specified radius."), nil
	}

	results := make([]stopSummary, 0, len(list))
	for _, item := range list {
		stop, ok := item.(map[string]any)
		if !ok {
			continue
		}
		results = append(results, stopFromMap(stop))
	}

	maxCount := int(req.GetFloat("max_count", 10))
	if maxCount < 1 {
		maxCount = 1
	}
	if maxCount > 20 {
		maxCount = 20
	}

	total := len(results)
	note := ""
	if total > maxCount {
		note = fmt.Sprintf(" (showing %d of %d in area)", maxCount, total)
		results = results[:maxCount]
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Found %d stops near (%.4f, %.4f)%s:\n%s", len(results), lat, lon, note, out)), nil
}

func (h *Handler) getStopsForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := req.RequireString("agency_id")
	if err != nil || agencyID == "" {
		return mcp.NewToolResultError("agency_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/stops-for-agency/"+agencyID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	results := make([]stopSummary, 0, len(list))
	for _, item := range list {
		stop, ok := item.(map[string]any)
		if !ok {
			continue
		}
		results = append(results, stopFromMap(stop))
	}

	total := len(results)
	note := ""
	if total > 50 {
		note = fmt.Sprintf(" (showing 50 of %d — use find_stops_near_location to filter by area)", total)
		results = results[:50]
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Stops for agency %s (%d shown%s):\n%s", agencyID, len(results), note, out)), nil
}

func (h *Handler) getStopSchedule(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := req.RequireString("stop_id")
	if err != nil || stopID == "" {
		return mcp.NewToolResultError("stop_id is required"), nil
	}

	params := url.Values{}
	if date := req.GetString("date", ""); date != "" {
		params.Set("date", date)
	}

	resp, err := h.client.Get("/api/where/schedule-for-stop/"+stopID+".json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText("No schedule data found for this stop."), nil
	}

	return mcp.NewToolResultText(formatStopSchedule(stopID, entry)), nil
}

type scheduledTrip struct {
	TripID    string `json:"trip_id"`
	Departure string `json:"departure"`
}

type directionSchedule struct {
	Headsign string           `json:"headsign"`
	Trips    []scheduledTrip  `json:"trips"`
}

type routeScheduleOutput struct {
	RouteID    string              `json:"route_id"`
	Directions []directionSchedule `json:"directions"`
}

type stopScheduleOutput struct {
	StopID string                `json:"stop_id"`
	Date   string                `json:"date"`
	Routes []routeScheduleOutput `json:"routes"`
}

// formatStopSchedule returns a structured schedule including trip IDs for every departure.
func formatStopSchedule(stopID string, entry map[string]any) string {
	dateMs := client.FloatVal(entry["date"])
	dateStr := ""
	if dateMs > 0 {
		dateStr = time.UnixMilli(int64(dateMs)).Format("2006-01-02")
	}

	out := stopScheduleOutput{StopID: stopID, Date: dateStr}

	for _, routeScheduleRaw := range client.AsSlice(entry["stopRouteSchedules"]) {
		routeSchedule, ok := routeScheduleRaw.(map[string]any)
		if !ok {
			continue
		}
		rs := routeScheduleOutput{RouteID: client.StrVal(routeSchedule["routeId"])}

		for _, dirRaw := range client.AsSlice(routeSchedule["stopRouteDirectionSchedules"]) {
			dir, ok := dirRaw.(map[string]any)
			if !ok {
				continue
			}
			ds := directionSchedule{Headsign: client.StrVal(dir["tripHeadsign"])}

			for _, stRaw := range client.AsSlice(dir["scheduleStopTimes"]) {
				st, ok := stRaw.(map[string]any)
				if !ok {
					continue
				}
				depMs := client.FloatVal(st["departureTime"])
				if depMs == 0 {
					continue
				}
				ds.Trips = append(ds.Trips, scheduledTrip{
					TripID:    client.StrVal(st["tripId"]),
					Departure: time.UnixMilli(int64(depMs)).Format("3:04 PM"),
				})
			}
			rs.Directions = append(rs.Directions, ds)
		}
		out.Routes = append(out.Routes, rs)
	}

	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b)
}

func (h *Handler) getStopIDsForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := req.RequireString("agency_id")
	if err != nil || agencyID == "" {
		return mcp.NewToolResultError("agency_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/stop-ids-for-agency/"+agencyID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	ids := make([]string, 0, len(list))
	for _, item := range list {
		ids = append(ids, fmt.Sprintf("%v", item))
	}

	total := len(ids)
	note := ""
	if total > 100 {
		note = fmt.Sprintf(" (showing 100 of %d)", total)
		ids = ids[:100]
	}
	out, _ := json.MarshalIndent(ids, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Stop IDs for agency %s (%d shown%s):\n%s", agencyID, len(ids), note, out)), nil
}
