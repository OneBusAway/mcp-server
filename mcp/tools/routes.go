package tools

import (
	"context"
	"fmt"
	"net/url"
	"oba-mcp/client"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerRouteTools(s *server.MCPServer) {
	s.AddTool(
		newPaginatedTool("get_route_ids_for_agency",
			mcp.WithDescription("Get a flat list of all route IDs for an agency. Use get_routes_for_agency if you also need names and details."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[string]]](),
		),
		h.getRouteIDsForAgency,
	)

	s.AddTool(
		mcp.NewTool("get_shape",
			mcp.WithDescription("Get the encoded polyline shape for a GTFS shape ID. Returns the path geometry a vehicle follows. Shape IDs come from trip details or stop-times data."),
			mcp.WithString("shape_id", mcp.Required(), mcp.Description("GTFS shape ID")),
			mcp.WithOutputSchema[SuccessEnvelope[ShapeResponse]](),
		),
		h.getShape,
	)

	s.AddTool(
		mcp.NewTool("get_route",
			mcp.WithDescription("Get details for a single route by ID: short name, long name, agency, color. Use when you already have a route_id."),
			mcp.WithString("route_id", mcp.Required(), mcp.Description("Route ID (e.g. 'unitrans_E')")),
			mcp.WithOutputSchema[SuccessEnvelope[RouteResponse]](),
		),
		h.getRoute,
	)

	s.AddTool(
		newPaginatedTool("search_routes",
			mcp.WithDescription("Search routes by number or name (e.g. 'E', 'express'). Returns matching route IDs and names. Use this to find a route_id when you only have a name."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Route number or name to search for")),
			mcp.WithNumber("max_count", mcp.Description("Max results to return: 1-20 (default: 5)")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[RouteResponse]]](),
		),
		h.searchRoutes,
	)

	s.AddTool(
		newPaginatedTool("get_routes_for_agency",
			mcp.WithDescription("List all routes operated by an agency. Use this when you have an agency_id from get_agencies — prefer this over get_routes_for_location when the agency is already known. Returns up to 30 by default."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[RouteResponse]]](),
		),
		h.getRoutesForAgency,
	)

	s.AddTool(
		newPaginatedTool("get_routes_for_location",
			mcp.WithDescription("Find routes that serve an area near GPS coordinates. Can return routes from multiple agencies serving the area — useful in multi-agency regions. Use get_routes_for_agency instead if you already have an agency_id, as it returns a complete unfiltered list."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters: 1-5000 (default: 500)")),
			mcp.WithNumber("lat_span", mcp.Description("Bounding-box latitude span: >0 and ≤180 (alternative to radius)")),
			mcp.WithNumber("lon_span", mcp.Description("Bounding-box longitude span: >0 and ≤180 (alternative to radius)")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[RouteResponse]]](),
		),
		h.getRoutesForLocation,
	)

	s.AddTool(
		mcp.NewTool("get_stops_for_route",
			mcp.WithDescription("Get all stops on a route, organized by direction (inbound/outbound)."),
			mcp.WithString("route_id", mcp.Required(), mcp.Description("Route ID (e.g. 'unitrans_E')")),
			mcp.WithOutputSchema[SuccessEnvelope[RouteStopsResponse]](),
		),
		h.getStopsForRoute,
	)

	s.AddTool(
		newPaginatedTool("get_trips_for_route",
			mcp.WithDescription("Get active trips currently running on a route, with real-time status and vehicle info."),
			mcp.WithString("route_id", mcp.Required(), mcp.Description("Route ID (e.g. 'unitrans_E')")),
			mcp.WithNumber("time", mcp.Description("Query trips at a Unix epoch millisecond timestamp through 2100")),
			mcp.WithBoolean("include_schedule", mcp.Description("Include full stop schedule in response (default true)")),
			mcp.WithBoolean("include_status", mcp.Description("Include real-time status in response (default true)")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[RouteTripResponse]]](),
		),
		h.getTripsForRoute,
	)

	s.AddTool(
		mcp.NewTool("get_schedule_for_route",
			mcp.WithDescription("Get the structure of a route's schedule for a date: directions, trip IDs, and stop order. Does not include per-stop departure times (use get_stop_schedule for a stop's timetable with times). Use this to answer 'what trips run on route E?' or 'how many trips does route L have?'"),
			mcp.WithString("route_id", mcp.Required(), mcp.Description("Route ID (e.g. 'unitrans_E')")),
			mcp.WithString("date", mcp.Description("Agency-local service date in strict YYYY-MM-DD format (defaults to today)")),
			mcp.WithOutputSchema[SuccessEnvelope[RouteScheduleStructureResponse]](),
		),
		h.getScheduleForRoute,
	)
}

func routeFromDTO(route client.Route) RouteResponse {
	return RouteResponse{
		ID: route.ID, ShortName: route.ShortName, LongName: route.LongName, AgencyID: route.AgencyID, Color: route.Color,
	}
}

func (h *Handler) getRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeID, err := entityIDArgument(req, "route_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.GetRoute(ctx, routeID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	if resp.Code != 200 || resp.Data.Entry.ID == "" {
		return toResult(textResult(fmt.Sprintf("No route found with ID %q.", routeID))), nil
	}

	return toResult(withCache(dataResult(fmt.Sprintf("Route %s:\n", routeID), routeFromDTO(resp.Data.Entry)), string(resp.CacheState))), nil
}

func (h *Handler) searchRoutes(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := searchArgument(req)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	maxCount, err := optionalLimit(req, "max_count", 5, 20)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "search_routes", maxCount)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	params := url.Values{
		"input":    {query},
		"maxCount": {fmt.Sprintf("%d", maximumPageSize)},
	}

	resp, err := h.client.SearchRoutes(ctx, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		return toResult(withCache(dataResult(fmt.Sprintf("No routes found matching %q.", query), Page[RouteResponse]{Items: []RouteResponse{}}), string(resp.CacheState))), nil
	}

	results := make([]RouteResponse, 0, len(list))
	for _, route := range list {
		results = append(results, routeFromDTO(route))
	}
	page, truncated := paginate("search_routes", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Found %d routes matching %q:\n", len(page.Items), query), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getRoutesForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := entityIDArgument(req, "agency_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_routes_for_agency", 30)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.RoutesForAgency(ctx, agencyID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	results := make([]RouteResponse, 0, len(list))
	for _, route := range list {
		results = append(results, routeFromDTO(route))
	}

	page, truncated := paginate("get_routes_for_agency", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Routes for agency %s (%d shown):\n", agencyID, len(page.Items)), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getRoutesForLocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lat, lon, err := requiredCoordinates(req)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_routes_for_location", 20)
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

	resp, err := h.client.RoutesForLocation(ctx, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		return toResult(withCache(dataResult("No routes found near that location.", Page[RouteResponse]{Items: []RouteResponse{}}), string(resp.CacheState))), nil
	}

	results := make([]RouteResponse, 0, len(list))
	for _, route := range list {
		results = append(results, routeFromDTO(route))
	}

	page, truncated := paginate("get_routes_for_location", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Found %d routes near (%.4f, %.4f):\n", len(page.Items), lat, lon), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getStopsForRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeID, err := entityIDArgument(req, "route_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.StopsForRoute(ctx, routeID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.RouteID == "" {
		return toResult(textResult("No stops found for this route.")), nil
	}

	stopLookup := map[string]client.Stop{}
	for _, stop := range resp.Data.References.Stops {
		stopLookup[stop.ID] = stop
	}

	var groups []RouteDirectionStopsResponse
	truncated := false
	for _, grouping := range entry.StopGroupings {
		for _, group := range grouping.StopGroups {
			stops := make([]StopResponse, 0, len(group.StopIDs))
			for _, id := range group.StopIDs {
				ref := stopLookup[id]
				stops = append(stops, StopResponse{ID: id, Name: ref.Name, Lat: ref.Lat, Lon: ref.Lon})
			}
			dirNote := ""
			if len(stops) > 30 {
				truncated = true
				dirNote = fmt.Sprintf(" [first 30 of %d]", len(stops))
				stops = stops[:30]
			}
			groups = append(groups, RouteDirectionStopsResponse{
				Direction: group.Name.Name + dirNote,
				Stops:     stops,
			})
		}
	}

	polylines := make([]RoutePolylineResponse, 0)
	for _, polyline := range entry.Polylines {
		if polyline.Points != "" {
			polylines = append(polylines, RoutePolylineResponse{
				Points: polyline.Points,
				Length: polyline.Length,
			})
		}
	}

	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Stops for route %s:\n", routeID), RouteStopsResponse{Directions: groups, Polylines: polylines}, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getTripsForRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeID, err := entityIDArgument(req, "route_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_trips_for_route", 20)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	params := url.Values{}
	if timestamp, present, err := optionalTimestamp(req, "time"); err != nil {
		return toResult(errorResult(err.Error())), nil
	} else if present {
		params.Set("time", fmt.Sprintf("%d", timestamp))
	}
	includeSchedule, err := optionalBool(req, "include_schedule", true)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	includeStatus, err := optionalBool(req, "include_status", true)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	params.Set("includeSchedule", fmt.Sprintf("%t", includeSchedule))
	params.Set("includeStatus", fmt.Sprintf("%t", includeStatus))

	resp, err := h.client.TripsForRoute(ctx, routeID, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		return toResult(withCache(dataResult(fmt.Sprintf("No active trips found for route %s.", routeID), Page[RouteTripResponse]{Items: []RouteTripResponse{}}), string(resp.CacheState))), nil
	}

	results := make([]RouteTripResponse, 0, len(list))
	for _, trip := range list {
		status := trip.Status
		active := resolveActiveTrip(status, resp.Data.References.Trips)
		if status == nil || active == nil || active.RouteID != routeID || strings.EqualFold(status.Status, "CANCELED") {
			continue
		}
		activeTripID := status.ActiveTripID
		if activeTripID == "" {
			activeTripID = active.ID
		}
		out := RouteTripResponse{TripID: activeTripID, RouteID: active.RouteID, ActiveTripID: activeTripID, ActiveRouteID: active.RouteID, ActiveShapeID: active.ShapeID, Headsign: active.TripHeadsign, Phase: status.Phase, VehicleID: status.VehicleID}
		if status.Position != nil {
			out.Lat = status.Position.Lat
			out.Lon = status.Position.Lon
		}
		results = append(results, out)
	}

	page, truncated := paginate("get_trips_for_route", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Active trips for route %s (%d shown):\n", routeID, len(page.Items)), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getScheduleForRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeID, err := entityIDArgument(req, "route_id")
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

	resp, err := h.client.ScheduleForRoute(ctx, routeID, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.RouteID == "" {
		return toResult(textResult("No schedule data found for this route.")), nil
	}

	loc := h.client.TimezoneFor(ctx, client.AgencyIDFromEntityID(routeID))
	result := RouteScheduleStructureResponse{RouteID: routeID, Timezone: loc.String()}
	if d := entry.ScheduleDate; d > 0 {
		result.DateMS = d
		result.DateDisplay = time.UnixMilli(int64(d)).In(loc).Format("2006-01-02")
	}

	for _, grp := range entry.StopTripGroupings {
		dir := RouteScheduleDirectionResponse{DirectionID: grp.DirectionID, TripIDs: append([]string(nil), grp.TripIDs...), StopIDs: append([]string(nil), grp.StopIDs...)}
		if len(grp.TripHeadsigns) > 0 {
			dir.Headsign = grp.TripHeadsigns[0]
		}
		dir.TripCount = len(dir.TripIDs)
		result.Directions = append(result.Directions, dir)
	}

	return toResult(withCache(dataResultWithSuffix(fmt.Sprintf("Route schedule structure for %s:\n", routeID), result, "\nFor departure times at a specific stop, use get_stop_schedule."), string(resp.CacheState))), nil
}

func (h *Handler) getRouteIDsForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := entityIDArgument(req, "agency_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_route_ids_for_agency", 50)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.RouteIDsForAgency(ctx, agencyID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	ids := RouteIDsResponse(append([]string(nil), resp.Data.List...))

	page, truncated := paginate("get_route_ids_for_agency", []string(ids), offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Route IDs for agency %s (%d shown):\n", agencyID, len(page.Items)), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getShape(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	shapeID, err := entityIDArgument(req, "shape_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.GetShape(ctx, shapeID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	if resp.Code != 200 || resp.Data.Entry.Points == "" {
		return toResult(textResult(fmt.Sprintf("No shape found with ID %q.", shapeID))), nil
	}

	return toResult(withCache(dataResult(fmt.Sprintf("Shape %s:\n", shapeID), ShapeResponse{Points: resp.Data.Entry.Points, Length: resp.Data.Entry.Length}), string(resp.CacheState))), nil
}
