package tools

import (
	"context"
	"fmt"
	"net/url"
	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerTripTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_block",
			mcp.WithDescription("Get the block configuration for a block ID — the ordered sequence of trips a vehicle makes in a single day. Block IDs come from trip details."),
			mcp.WithString("block_id", mcp.Required(), mcp.Description("Block ID (from a trip's blockId field)")),
			mcp.WithOutputSchema[SuccessEnvelope[BlockResponse]](),
		),
		h.getBlock,
	)

	s.AddTool(
		mcp.NewTool("get_trip",
			mcp.WithDescription("Get static info for a trip by ID: route, headsign, direction, shape, and service dates. Does NOT include real-time vehicle position or schedule deviation — use get_trip_details for live status."),
			mcp.WithString("trip_id", mcp.Required(), mcp.Description("Trip ID (e.g. 'unitrans_12345')")),
			mcp.WithOutputSchema[SuccessEnvelope[TripResponse]](),
		),
		h.getTrip,
	)

	s.AddTool(
		mcp.NewTool("get_trip_details",
			mcp.WithDescription("Get full real-time status for a trip independently of a stop: vehicle position, schedule deviation, next stops, and phase. Do not use this to refresh a selected arrival at a known stop; use get_arrival_and_departure_for_stop for that tracking flow. Requires a real trip_id explicitly supplied by the user or returned by another tool; never substitute a stop_id or route_id, and do not guess one."),
			mcp.WithString("trip_id", mcp.Required(), mcp.Description("Trip ID returned by a prior tool or explicitly provided by the user; never use a stop ID or route ID")),
			mcp.WithNumber("time", mcp.Description("Query trip at a Unix epoch millisecond timestamp through 2100")),
			mcp.WithBoolean("include_schedule", mcp.Description("Include full stop schedule in response (default true)")),
			mcp.WithBoolean("include_status", mcp.Description("Include real-time status in response (default true)")),
			mcp.WithOutputSchema[SuccessEnvelope[TripDetailsResponse]](),
		),
		h.getTripDetails,
	)

	s.AddTool(
		mcp.NewTool("get_trip_for_vehicle",
			mcp.WithDescription("Get the current trip being served by a specific vehicle. Use when the user asks about a specific bus by vehicle number. Returns nothing if the vehicle has no active trip or is not currently tracked by the real-time feed."),
			mcp.WithString("vehicle_id", mcp.Required(), mcp.Description("Vehicle ID (e.g. 'unitrans_1')")),
			mcp.WithNumber("time", mcp.Description("Query trip at a Unix epoch millisecond timestamp through 2100")),
			mcp.WithOutputSchema[SuccessEnvelope[VehicleResponse]](),
		),
		h.getTripForVehicle,
	)

	s.AddTool(
		newPaginatedTool("get_vehicles_for_agency",
			mcp.WithDescription("Get real-time positions and status of every active vehicle for an agency. Use ONLY when the user explicitly asks for the whole fleet or all buses running. Returns an empty list if no real-time feed is configured for the agency. For buses approaching a specific stop, use get_arrivals_for_stop instead."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[VehicleResponse]]](),
		),
		h.getVehiclesForAgency,
	)

	s.AddTool(
		newPaginatedTool("get_trips_for_location",
			mcp.WithDescription("Get active trips near GPS coordinates. Useful for finding buses currently operating in an area."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters: 1-5000 (default: 500)")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[VehicleResponse]]](),
		),
		h.getTripsForLocation,
	)
}

func tripDetailsResponse(entry client.TripDetails, references []client.Trip, requestedID string) TripDetailsResponse {
	response := TripDetailsResponse{TripID: entry.TripID}
	if response.TripID == "" {
		response.TripID = requestedID
	}
	trip := entry.Trip
	if trip == nil {
		for _, reference := range references {
			if reference.ID == response.TripID {
				value := reference
				trip = &value
				break
			}
		}
	}
	if trip != nil {
		response.RouteID = trip.RouteID
		response.ShapeID = trip.ShapeID
	}
	if entry.Status != nil {
		status := entry.Status
		active := resolveActiveTrip(status, references)
		response.Phase = status.Phase
		response.Status = status.Status
		response.VehicleID = status.VehicleID
		response.DeviationMins = status.ScheduleDeviation / 60
		response.Bearing = status.Orientation
		response.ActiveTripID = status.ActiveTripID
		if active != nil {
			if response.ActiveTripID == "" {
				response.ActiveTripID = active.ID
			}
			response.ActiveRouteID = active.RouteID
			response.ActiveShapeID = active.ShapeID
		}
		if status.Position != nil {
			response.Lat = status.Position.Lat
			response.Lon = status.Position.Lon
		}
	}
	return response
}

func resolveActiveTrip(status *client.TripStatus, references []client.Trip) *client.Trip {
	if status == nil {
		return nil
	}
	if status.ActiveTrip != nil {
		return status.ActiveTrip
	}
	for i := range references {
		if references[i].ID == status.ActiveTripID {
			return &references[i]
		}
	}
	return nil
}

func vehicleResponse(entry client.TripDetails, references []client.Trip) VehicleResponse {
	detail := tripDetailsResponse(entry, references, entry.TripID)
	tripID := detail.ActiveTripID
	if tripID == "" {
		tripID = detail.TripID
	}
	routeID := detail.ActiveRouteID
	if routeID == "" {
		routeID = detail.RouteID
	}
	return VehicleResponse{VehicleID: detail.VehicleID, TripID: tripID, RouteID: routeID, ActiveTripID: detail.ActiveTripID, ActiveRouteID: detail.ActiveRouteID, ActiveShapeID: detail.ActiveShapeID, Phase: detail.Phase, Lat: detail.Lat, Lon: detail.Lon, DeviationMins: detail.DeviationMins}
}

func vehicleStatusResponse(vehicle client.VehicleStatus, references []client.Trip) VehicleResponse {
	response := VehicleResponse{VehicleID: vehicle.VehicleID, TripID: vehicle.TripID, Phase: vehicle.Phase}
	if vehicle.Location != nil {
		response.Lat = vehicle.Location.Lat
		response.Lon = vehicle.Location.Lon
	}
	if vehicle.TripStatus != nil {
		status := vehicle.TripStatus
		active := resolveActiveTrip(status, references)
		response.ActiveTripID = status.ActiveTripID
		if response.ActiveTripID == "" && active != nil {
			response.ActiveTripID = active.ID
		}
		if response.ActiveTripID != "" {
			response.TripID = response.ActiveTripID
		}
		response.DeviationMins = status.ScheduleDeviation / 60
		if active != nil {
			response.ActiveRouteID = active.RouteID
			response.ActiveShapeID = active.ShapeID
			response.RouteID = response.ActiveRouteID
			response.Headsign = active.TripHeadsign
		}
	}
	return response
}

func (h *Handler) getTrip(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tripID, err := entityIDArgument(req, "trip_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.GetTrip(ctx, tripID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.ID == "" {
		return toResult(textResult(fmt.Sprintf("No trip found with ID %q.", tripID))), nil
	}

	return toResult(withCache(dataResult(fmt.Sprintf("Trip %s:\n", tripID), TripResponse{ID: entry.ID, RouteID: entry.RouteID, Headsign: entry.TripHeadsign, DirectionID: entry.DirectionID, ServiceID: entry.ServiceID}), string(resp.CacheState))), nil
}

func (h *Handler) getTripDetails(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tripID, err := entityIDArgument(req, "trip_id")
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

	resp, err := h.client.TripDetails(ctx, tripID, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.TripID == "" {
		return toResult(textResult("No trip found with that ID.")), nil
	}

	detail := tripDetailsResponse(entry, resp.Data.References.Trips, tripID)

	return toResult(withCache(dataResult(fmt.Sprintf("Trip details for %s:\n", tripID), detail), string(resp.CacheState))), nil
}

func (h *Handler) getTripForVehicle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	vehicleID, err := entityIDArgument(req, "vehicle_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	params := url.Values{}
	if timestamp, present, err := optionalTimestamp(req, "time"); err != nil {
		return toResult(errorResult(err.Error())), nil
	} else if present {
		params.Set("time", fmt.Sprintf("%d", timestamp))
	}

	resp, err := h.client.TripForVehicle(ctx, vehicleID, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.TripID == "" {
		return toResult(textResult(fmt.Sprintf("No active trip found for vehicle %q.", vehicleID))), nil
	}

	v := vehicleResponse(entry, resp.Data.References.Trips)

	return toResult(withCache(dataResult(fmt.Sprintf("Current trip for vehicle %s:\n", vehicleID), v), string(resp.CacheState))), nil
}

func (h *Handler) getVehiclesForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := entityIDArgument(req, "agency_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_vehicles_for_agency", 50)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.VehiclesForAgency(ctx, agencyID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		return toResult(withCache(dataResult(fmt.Sprintf("No active vehicles for agency %s.", agencyID), Page[VehicleResponse]{Items: []VehicleResponse{}}), string(resp.CacheState))), nil
	}

	results := make([]VehicleResponse, 0, len(list))
	for _, vehicle := range list {
		results = append(results, vehicleStatusResponse(vehicle, resp.Data.References.Trips))
	}

	page, truncated := paginate("get_vehicles_for_agency", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Active vehicles for agency %s (%d shown):\n", agencyID, len(page.Items)), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getTripsForLocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lat, lon, err := requiredCoordinates(req)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_trips_for_location", 20)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	radius, err := optionalRadius(req, 500, 5_000)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	params := url.Values{
		"lat":    {fmt.Sprintf("%f", lat)},
		"lon":    {fmt.Sprintf("%f", lon)},
		"radius": {fmt.Sprintf("%d", radius)},
	}

	resp, err := h.client.TripsForLocation(ctx, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		return toResult(withCache(dataResult("No active trips found near that location.", Page[VehicleResponse]{Items: []VehicleResponse{}}), string(resp.CacheState))), nil
	}

	results := make([]VehicleResponse, 0, len(list))
	for _, trip := range list {
		results = append(results, vehicleResponse(client.TripDetails{TripID: trip.TripID, Status: trip.Status, Trip: trip.Trip}, nil))
	}

	page, truncated := paginate("get_trips_for_location", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Active trips near (%.4f, %.4f) — %d shown:\n", lat, lon, len(page.Items)), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getBlock(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	blockID, err := entityIDArgument(req, "block_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.GetBlock(ctx, blockID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.ID == "" {
		return toResult(textResult(fmt.Sprintf("No block found with ID %q.", blockID))), nil
	}

	output := BlockResponse{ID: entry.ID}
	for _, config := range entry.Configurations {
		response := BlockConfigurationResponse{ActiveServiceIDs: append([]string(nil), config.ActiveServiceIDs...)}
		for _, trip := range config.Trips {
			item := BlockTripResponse{TripID: trip.TripID}
			if trip.Trip != nil {
				item.RouteID = trip.Trip.RouteID
				item.Headsign = trip.Trip.TripHeadsign
			}
			response.Trips = append(response.Trips, item)
		}
		output.Configurations = append(output.Configurations, response)
	}
	return toResult(withCache(dataResult(fmt.Sprintf("Block %s configurations:\n", blockID), output), string(resp.CacheState))), nil
}
