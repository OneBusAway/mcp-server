package tools

import (
	"context"
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
			mcp.WithNumber("max_count", mcp.Description("Max results to return: 1-20 (default: 5)")),
		),
		h.searchStops,
	)

	s.AddTool(
		mcp.NewTool("find_stops_near_location",
			mcp.WithDescription("Find transit stops near GPS coordinates. Returns nearby stop IDs, names, directions, and routes."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters (default: 500, max: 5000)")),
			mcp.WithNumber("lat_span", mcp.Description("Bounding-box latitude span: >0 and ≤180 (alternative to radius)")),
			mcp.WithNumber("lon_span", mcp.Description("Bounding-box longitude span: >0 and ≤180 (alternative to radius)")),
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
			mcp.WithString("date", mcp.Description("Agency-local service date in strict YYYY-MM-DD format (defaults to today)")),
		),
		h.getStopSchedule,
	)
}

func stopFromDTO(stop client.Stop) StopResponse {
	return StopResponse{ID: stop.ID, Name: stop.Name, Code: stop.Code, Direction: stop.Direction, Lat: stop.Lat, Lon: stop.Lon, RouteIDs: append([]string(nil), stop.RouteIDs...)}
}

func (h *Handler) getStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := entityIDArgument(req, "stop_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.GetStop(ctx, stopID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	if resp.Code != 200 || resp.Data.Entry.ID == "" {
		return toResult(textResult(fmt.Sprintf("No stop found with ID %q.", stopID))), nil
	}

	return toResult(dataResult(fmt.Sprintf("Stop %s:\n", stopID), stopFromDTO(resp.Data.Entry))), nil
}

func (h *Handler) searchStops(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := searchArgument(req)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	maxCount, err := optionalLimit(req, "max_count", 5, 20)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	params := url.Values{
		"input":    {query},
		"maxCount": {fmt.Sprintf("%d", maxCount)},
	}

	resp, err := h.client.SearchStops(ctx, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		return toResult(textResult(fmt.Sprintf("No stops found matching %q.", query))), nil
	}

	results := make([]StopResponse, 0, len(list))
	for _, stop := range list {
		results = append(results, stopFromDTO(stop))
	}
	if len(results) > maxCount {
		results = results[:maxCount]
	}

	return toResult(dataResult(fmt.Sprintf("Found %d stops matching %q:\n", len(results), query), results)), nil
}

func (h *Handler) findStopsNearLocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lat, lon, err := requiredCoordinates(req)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	radius, err := optionalRadius(req, 500, 5_000)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	latSpan, hasLatSpan, err := optionalSpan(req, "lat_span")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	lonSpan, hasLonSpan, err := optionalSpan(req, "lon_span")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	maxCount, err := optionalLimit(req, "max_count", 10, 20)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	_, radiusProvided, err := numberArgument(req, "radius", false)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	if hasLatSpan != hasLonSpan {
		return toResult(errorResult("lat_span and lon_span must be provided together")), nil
	}
	if hasLatSpan && radiusProvided {
		return toResult(errorResult("radius cannot be combined with lat_span and lon_span")), nil
	}
	params := url.Values{
		"lat": {fmt.Sprintf("%f", lat)},
		"lon": {fmt.Sprintf("%f", lon)},
	}
	if hasLatSpan {
		params.Set("latSpan", fmt.Sprintf("%f", latSpan))
		params.Set("lonSpan", fmt.Sprintf("%f", lonSpan))
	} else {
		params.Set("radius", fmt.Sprintf("%d", radius))
	}

	resp, err := h.client.StopsForLocation(ctx, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		return toResult(textResult("No stops found within the specified radius.")), nil
	}

	results := make([]StopResponse, 0, len(list))
	for _, stop := range list {
		results = append(results, stopFromDTO(stop))
	}

	total := len(results)
	note := ""
	if total > maxCount {
		note = fmt.Sprintf(" (showing %d of %d in area)", maxCount, total)
		results = results[:maxCount]
	}
	return toResult(dataResult(fmt.Sprintf("Found %d stops near (%.4f, %.4f)%s:\n", len(results), lat, lon, note), results)), nil
}

func (h *Handler) getStopsForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := entityIDArgument(req, "agency_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.StopsForAgency(ctx, agencyID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	results := make([]StopResponse, 0, len(list))
	for _, stop := range list {
		results = append(results, stopFromDTO(stop))
	}

	total := len(results)
	note := ""
	if total > 50 {
		note = fmt.Sprintf(" (showing 50 of %d — use find_stops_near_location to filter by area)", total)
		results = results[:50]
	}
	return toResult(dataResult(fmt.Sprintf("Stops for agency %s (%d shown%s):\n", agencyID, len(results), note), results)), nil
}

func (h *Handler) getStopSchedule(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := entityIDArgument(req, "stop_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	params := url.Values{}
	date, err := optionalDate(req)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	if date != "" {
		params.Set("date", date)
	}

	resp, err := h.client.ScheduleForStop(ctx, stopID, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	if resp.Code != 200 || resp.Data.Entry.StopID == "" {
		return toResult(textResult("No schedule data found for this stop.")), nil
	}

	return toResult(dataResult("", stopScheduleResponse(stopID, resp.Data.Entry))), nil
}

func stopScheduleResponse(stopID string, entry client.ScheduleForStop) StopScheduleResponse {
	dateMs := entry.Date
	dateStr := ""
	if dateMs > 0 {
		dateStr = time.UnixMilli(int64(dateMs)).Format("2006-01-02")
	}

	out := StopScheduleResponse{StopID: stopID, Date: dateStr}

	for _, routeSchedule := range entry.StopRouteSchedules {
		rs := RouteScheduleResponse{RouteID: routeSchedule.RouteID}

		for _, dir := range routeSchedule.StopRouteDirectionSchedules {
			ds := DirectionScheduleResponse{Headsign: dir.TripHeadsign}

			for _, st := range dir.ScheduleStopTimes {
				depMs := st.DepartureTime
				if depMs == 0 {
					continue
				}
				ds.Trips = append(ds.Trips, ScheduledTripResponse{
					TripID:    st.TripID,
					Departure: time.UnixMilli(int64(depMs)).Format("3:04 PM"),
				})
			}
			rs.Directions = append(rs.Directions, ds)
		}
		out.Routes = append(out.Routes, rs)
	}

	return out
}

func (h *Handler) getStopIDsForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := entityIDArgument(req, "agency_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.StopIDsForAgency(ctx, agencyID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	ids := StopIDsResponse(append([]string(nil), resp.Data.List...))

	total := len(ids)
	note := ""
	if total > 100 {
		note = fmt.Sprintf(" (showing 100 of %d)", total)
		ids = ids[:100]
	}
	return toResult(dataResult(fmt.Sprintf("Stop IDs for agency %s (%d shown%s):\n", agencyID, len(ids), note), ids)), nil
}
