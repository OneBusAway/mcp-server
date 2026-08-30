package tools

// RouteResponse is returned by route lookup and route-list tools.
type RouteResponse struct {
	ID        string `json:"id"`
	ShortName string `json:"short_name"`
	LongName  string `json:"long_name,omitempty"`
	AgencyID  string `json:"agency_id"`
	Color     string `json:"color,omitempty"`
}

type RouteIDsResponse []string

// RouteStopsResponse is returned by get_stops_for_route.
type RouteDirectionStopsResponse struct {
	Direction string         `json:"direction"`
	Stops     []StopResponse `json:"stops"`
}

type RoutePolylineResponse struct {
	Points string  `json:"encoded_polyline"`
	Length float64 `json:"length,omitempty"`
}

type RouteStopsResponse struct {
	Directions []RouteDirectionStopsResponse `json:"directions"`
	Polylines  []RoutePolylineResponse       `json:"polylines,omitempty"`
}

type RouteTripResponse struct {
	TripID    string  `json:"trip_id"`
	Headsign  string  `json:"headsign,omitempty"`
	Phase     string  `json:"phase,omitempty"`
	VehicleID string  `json:"vehicle_id,omitempty"`
	Lat       float64 `json:"lat,omitempty"`
	Lon       float64 `json:"lon,omitempty"`
}

type RouteScheduleDirectionResponse struct {
	DirectionID string   `json:"direction_id"`
	Headsign    string   `json:"headsign"`
	TripCount   int      `json:"trip_count"`
	TripIDs     []string `json:"trip_ids"`
	StopIDs     []string `json:"stop_ids"`
}

type RouteScheduleStructureResponse struct {
	RouteID    string                           `json:"route_id"`
	Date       string                           `json:"date,omitempty"`
	Directions []RouteScheduleDirectionResponse `json:"directions"`
}

type ShapeResponse struct {
	Points string  `json:"encoded_polyline"`
	Length float64 `json:"length,omitempty"`
}
