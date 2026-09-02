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
		newPaginatedTool("get_stop_ids_for_agency",
			mcp.WithDescription("Get a flat list of all stop IDs for an agency. Useful when you need to enumerate stop IDs programmatically. Use get_stops_for_agency if you also need names and locations."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[string]]](),
		),
		h.getStopIDsForAgency,
	)

	s.AddTool(
		mcp.NewTool("get_stop",
			mcp.WithDescription("Look up a stop by ID and return its name, location, direction, and route list. Use this when you already have a stop_id — faster than searching. IDs look like 'unitrans_22274'. Use get_arrivals_for_stop to see upcoming departures at this stop."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
			mcp.WithOutputSchema[SuccessEnvelope[StopResponse]](),
		),
		h.getStop,
	)

	s.AddTool(
		newPaginatedTool("search_stops",
			mcp.WithDescription("Search stops by name, street, stop code, or landmark text. This is the primary way to translate a user-facing stop code (the short number printed on the physical sign, e.g. '1013') into the underlying agency-specific stop_id. Do not guess or construct a stop_id from a code — search for it. If the user names a landmark but gives no numeric coordinates, use this tool with the landmark text; do not invent GPS coordinates. Returns matching stops with their IDs, names, and coordinates."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Stop name, street, stop code, or landmark text to search for")),
			mcp.WithNumber("max_count", mcp.Description("Max results to return: 1-20 (default: 5)")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[StopResponse]]](),
		),
		h.searchStops,
	)

	s.AddTool(
		newPaginatedTool("find_stops_near_location",
			mcp.WithDescription("Find transit stops near GPS coordinates. Requires actual numeric latitude and longitude provided by the user or derived from a prior lookup. Never geocode a landmark or infer coordinates: use search_stops when you only have a place name or text description."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters (default: 500, max: 5000)")),
			mcp.WithNumber("lat_span", mcp.Description("Bounding-box latitude span: >0 and ≤180 (alternative to radius)")),
			mcp.WithNumber("lon_span", mcp.Description("Bounding-box longitude span: >0 and ≤180 (alternative to radius)")),
			mcp.WithNumber("max_count", mcp.Description("Max stops to return (default: 10, max: 20). Pass the number the user asked for.")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[StopResponse]]](),
		),
		h.findStopsNearLocation,
	)

	s.AddTool(
		newPaginatedTool("get_stops_for_agency",
			mcp.WithDescription("List stops operated by an agency. Returns up to 50 by default — use find_stops_near_location to filter by area."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[StopResponse]]](),
		),
		h.getStopsForAgency,
	)

	s.AddTool(
		mcp.NewTool("get_stop_schedule",
			mcp.WithDescription("Get the full static day schedule for a stop grouped by route and direction, with trip_id and departure time per trip. Use for planning ahead (e.g. 'what buses run tomorrow?'). For real-time data or a rolling time window, use get_arrivals_for_stop instead — some trips may be missing here if the backend omits a direction grouping."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
			mcp.WithString("date", mcp.Description("Agency-local service date in strict YYYY-MM-DD format (defaults to today)")),
			mcp.WithOutputSchema[SuccessEnvelope[StopScheduleResponse]](),
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

	return toResult(withCache(dataResult(fmt.Sprintf("Stop %s:\n", stopID), stopFromDTO(resp.Data.Entry)), string(resp.CacheState))), nil
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
	offset, limit, err := pageArguments(req, "search_stops", maxCount)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	params := url.Values{
		"input":    {query},
		"maxCount": {fmt.Sprintf("%d", maximumPageSize)},
	}

	resp, err := h.client.SearchStops(ctx, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		return toResult(withCache(dataResult(fmt.Sprintf("No stops found matching %q.", query), Page[StopResponse]{Items: []StopResponse{}}), string(resp.CacheState))), nil
	}

	results := make([]StopResponse, 0, len(list))
	for _, stop := range list {
		results = append(results, stopFromDTO(stop))
	}
	page, truncated := paginate("search_stops", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Found %d stops matching %q:\n", len(page.Items), query), page, truncated), string(resp.CacheState))), nil
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
	offset, limit, err := pageArguments(req, "find_stops_near_location", maxCount)
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
		return toResult(withCache(dataResult("No stops found within the specified radius.", Page[StopResponse]{Items: []StopResponse{}}), string(resp.CacheState))), nil
	}

	results := make([]StopResponse, 0, len(list))
	for _, stop := range list {
		results = append(results, stopFromDTO(stop))
	}

	page, truncated := paginate("find_stops_near_location", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Found %d stops near (%.4f, %.4f):\n", len(page.Items), lat, lon), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getStopsForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := entityIDArgument(req, "agency_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_stops_for_agency", 50)
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

	page, truncated := paginate("get_stops_for_agency", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Stops for agency %s (%d shown):\n", agencyID, len(page.Items)), page, truncated), string(resp.CacheState))), nil
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

	loc := h.client.TimezoneFor(ctx, client.AgencyIDFromEntityID(stopID))
	return toResult(withCache(dataResult("", stopScheduleResponse(stopID, resp.Data.Entry, loc)), string(resp.CacheState))), nil
}

func stopScheduleResponse(stopID string, entry client.ScheduleForStop, loc *time.Location) StopScheduleResponse {
	if loc == nil {
		loc = time.UTC
	}
	dateMs := entry.Date
	dateStr := ""
	if dateMs > 0 {
		dateStr = time.UnixMilli(int64(dateMs)).In(loc).Format("2006-01-02")
	}

	out := StopScheduleResponse{StopID: stopID, DateMS: dateMs, DateDisplay: dateStr, Timezone: loc.String()}

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
					TripID:           st.TripID,
					DepartureMS:      depMs,
					DepartureDisplay: time.UnixMilli(int64(depMs)).In(loc).Format("3:04 PM"),
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
	offset, limit, err := pageArguments(req, "get_stop_ids_for_agency", 50)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.StopIDsForAgency(ctx, agencyID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	ids := StopIDsResponse(append([]string(nil), resp.Data.List...))

	page, truncated := paginate("get_stop_ids_for_agency", []string(ids), offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Stop IDs for agency %s (%d shown):\n", agencyID, len(page.Items)), page, truncated), string(resp.CacheState))), nil
}
