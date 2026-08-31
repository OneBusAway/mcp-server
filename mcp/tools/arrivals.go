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

func (h *Handler) registerArrivalTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_arrival_and_departure_for_stop",
			mcp.WithDescription("Get a single specific arrival/departure at a stop for a known trip. Use this when you have a specific trip_id and service_date (from get_arrivals_for_stop results) and need precise real-time data for that one trip."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
			mcp.WithString("trip_id", mcp.Required(), mcp.Description("Trip ID (e.g. 'unitrans_12345')")),
			mcp.WithNumber("service_date", mcp.Required(), mcp.Description("Service date as a Unix epoch millisecond timestamp through 2100")),
			mcp.WithString("vehicle_id", mcp.Description("Optional vehicle ID to disambiguate")),
			mcp.WithOutputSchema[SuccessEnvelope[ArrivalResponse]](),
		),
		h.getArrivalAndDepartureForStop,
	)

	s.AddTool(
		newPaginatedTool("get_arrivals_for_stop",
			mcp.WithDescription("Get arrivals and departures at a stop. Returns up to 10 arrivals in the next 30 minutes by default. Pass 'time' (epoch ms) to query at a specific moment (e.g. 7 AM). Pass 'minutes_after' to widen or narrow the window. Real-time predictions only available near current time."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
			mcp.WithNumber("minutes_before", mcp.Description("Minutes in the past to include: 0-120 (default: 0)")),
			mcp.WithNumber("minutes_after", mcp.Description("Minutes ahead to include: 0-120 (default: 30)")),
			mcp.WithNumber("time", mcp.Description("Optional Unix epoch millisecond timestamp through 2100; defaults to now")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[ArrivalResponse]]](),
		),
		h.getArrivalsForStop,
	)

	s.AddTool(
		newPaginatedTool("get_arrivals_for_location",
			mcp.WithDescription("Get real-time arrivals and departures for all stops within a radius of GPS coordinates. Returns the same arrival detail as get_arrivals_for_stop but for every nearby stop at once. Useful for 'what buses are near me right now?' Returns up to 10 arrivals."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters: 1-5000 (default: 500)")),
			mcp.WithNumber("minutes_before", mcp.Description("Minutes in the past to include: 0-120 (default: 0)")),
			mcp.WithNumber("minutes_after", mcp.Description("Minutes ahead to include: 0-120 (default: 35)")),
			mcp.WithNumber("time", mcp.Description("Optional Unix epoch millisecond timestamp through 2100; defaults to now")),
			mcp.WithOutputSchema[SuccessEnvelope[Page[ArrivalResponse]]](),
		),
		h.getArrivalsForLocation,
	)
}

func arrivalResponse(arrival client.ArrivalAndDeparture, loc *time.Location) ArrivalResponse {
	if loc == nil {
		loc = time.UTC
	}
	result := ArrivalResponse{
		TripID:               arrival.TripID,
		ServiceDateMS:        arrival.ServiceDate,
		RouteID:              arrival.RouteID,
		RouteName:            arrival.RouteShortName,
		Headsign:             arrival.TripHeadsign,
		Status:               arrival.Status,
		VehicleID:            arrival.VehicleID,
		Predicted:            arrival.Predicted,
		DistanceFromStop:     arrival.DistanceFromStop,
		Timezone:             loc.String(),
		ScheduledArrivalMS:   arrival.ScheduledArrivalTime,
		ScheduledDepartureMS: arrival.ScheduledDepartureTime,
	}
	// Pass through stops_away only when OBA explicitly supplied it.
	// A nil pointer means the field was absent in the API response; a pointer
	// to 0 means the bus is genuinely at the stop. Merging these would cause
	// the tracker to fire an "arriving" notification for trips where the field
	// was simply omitted.
	result.NumberOfStopsAway = arrival.NumberOfStopsAway
	if arrival.ScheduledArrivalTime > 0 {
		result.ScheduledArrivalDisplay = client.FormatRelativeTime(float64(arrival.ScheduledArrivalTime), loc)
	}
	if arrival.ScheduledDepartureTime > 0 {
		result.ScheduledDepartureDisplay = client.FormatRelativeTime(float64(arrival.ScheduledDepartureTime), loc)
	}
	if result.Predicted {
		result.PredictedArrivalMS = arrival.PredictedArrivalTime
		result.PredictedDepartureMS = arrival.PredictedDepartureTime
		if arrival.PredictedArrivalTime > 0 {
			result.PredictedArrivalDisplay = client.FormatRelativeTime(float64(arrival.PredictedArrivalTime), loc)
		}
		if arrival.PredictedDepartureTime > 0 {
			result.PredictedDepartureDisplay = client.FormatRelativeTime(float64(arrival.PredictedDepartureTime), loc)
		}
		if arrival.TripStatus != nil {
			result.VehicleBearing = arrival.TripStatus.Orientation
			if arrival.TripStatus.Position != nil {
				result.VehicleLat = arrival.TripStatus.Position.Lat
				result.VehicleLon = arrival.TripStatus.Position.Lon
			}
		}
	}
	return result
}

func arrivalResponses(arrivals []client.ArrivalAndDeparture, loc *time.Location) []ArrivalResponse {
	results := make([]ArrivalResponse, 0, len(arrivals))
	for _, arrival := range arrivals {
		results = append(results, arrivalResponse(arrival, loc))
	}
	return results
}

func (h *Handler) getArrivalsForStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := entityIDArgument(req, "stop_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_arrivals_for_stop", 10)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	loc := h.client.TimezoneFor(ctx, client.AgencyIDFromEntityID(stopID))

	minutesBefore, err := optionalWindow(req, "minutes_before", 0, 120)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	minutesAfter, err := optionalWindow(req, "minutes_after", 30, 120)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	params := url.Values{"minutesBefore": {fmt.Sprintf("%d", minutesBefore)}, "minutesAfter": {fmt.Sprintf("%d", minutesAfter)}}
	if timestamp, present, err := optionalTimestamp(req, "time"); err != nil {
		return toResult(errorResult(err.Error())), nil
	} else if present {
		params.Set("time", fmt.Sprintf("%d", timestamp))
	}

	resp, err := h.client.ArrivalsForStop(ctx, stopID, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.StopID == "" {
		return toResult(textResult("No arrival data found for this stop.")), nil
	}

	if len(entry.ArrivalsAndDepartures) == 0 {
		return toResult(withCache(dataResult(fmt.Sprintf("No upcoming arrivals at stop %s.", stopID), Page[ArrivalResponse]{Items: []ArrivalResponse{}}), string(resp.CacheState))), nil
	}

	results := arrivalResponses(entry.ArrivalsAndDepartures, loc)

	page, truncated := paginate("get_arrivals_for_stop", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Arrivals at stop %s (%d shown):\n", stopID, len(page.Items)), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getArrivalAndDepartureForStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := entityIDArgument(req, "stop_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	tripID, err := entityIDArgument(req, "trip_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	serviceDate, err := requiredTimestamp(req, "service_date")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	loc := h.client.TimezoneFor(ctx, client.AgencyIDFromEntityID(stopID))

	params := url.Values{
		"tripId":      {tripID},
		"serviceDate": {fmt.Sprintf("%d", serviceDate)},
	}
	if vehicleID, err := optionalEntityID(req, "vehicle_id"); err != nil {
		return toResult(errorResult(err.Error())), nil
	} else if vehicleID != "" {
		params.Set("vehicleId", vehicleID)
	}

	resp, err := h.client.ArrivalForStop(ctx, stopID, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 || entry.TripID == "" {
		return toResult(textResult("No arrival found for that trip at this stop.")), nil
	}

	ar := arrivalResponse(entry, loc)

	return toResult(withCache(dataResult(fmt.Sprintf("Arrival for trip %s at stop %s:\n", tripID, stopID), ar), string(resp.CacheState))), nil
}

func (h *Handler) getArrivalsForLocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lat, lon, err := requiredCoordinates(req)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	offset, limit, err := pageArguments(req, "get_arrivals_for_location", 10)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	radius, err := optionalRadius(req, 500, 5_000)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	minutesBefore, err := optionalWindow(req, "minutes_before", 0, 120)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	minutesAfter, err := optionalWindow(req, "minutes_after", 35, 120)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	params := url.Values{
		"lat":           {fmt.Sprintf("%f", lat)},
		"lon":           {fmt.Sprintf("%f", lon)},
		"maxCount":      {fmt.Sprintf("%d", maximumPageSize)},
		"radius":        {fmt.Sprintf("%d", radius)},
		"minutesBefore": {fmt.Sprintf("%d", minutesBefore)},
		"minutesAfter":  {fmt.Sprintf("%d", minutesAfter)},
	}
	if timestamp, present, err := optionalTimestamp(req, "time"); err != nil {
		return toResult(errorResult(err.Error())), nil
	} else if present {
		params.Set("time", fmt.Sprintf("%d", timestamp))
	}

	resp, err := h.client.ArrivalsForLocation(ctx, params)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	entry := resp.Data.Entry
	if resp.Code != 200 {
		return toResult(textResult("No arrival data found near that location.")), nil
	}

	if len(entry.ArrivalsAndDepartures) == 0 {
		return toResult(withCache(dataResult("No upcoming arrivals near that location.", Page[ArrivalResponse]{Items: []ArrivalResponse{}}), string(resp.CacheState))), nil
	}

	var loc = h.client.TimezoneFor(ctx, "")
	for _, arrival := range entry.ArrivalsAndDepartures {
		if agencyID := client.AgencyIDFromEntityID(arrival.RouteID); agencyID != "" {
			loc = h.client.TimezoneFor(ctx, agencyID)
			break
		}
	}
	results := arrivalResponses(entry.ArrivalsAndDepartures, loc)
	page, truncated := paginate("get_arrivals_for_location", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Arrivals near (%.4f, %.4f) — %d shown:\n", lat, lon, len(page.Items)), page, truncated), string(resp.CacheState))), nil
}
