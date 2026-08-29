// Package tools defines the MCP tool handlers and their public responses.
package tools

// AgencySummary is the public response item returned by get_agencies.
// These types define the MCP-facing contract, independently of OBA's raw API
// response shape.
type AgencySummary struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	URL  string  `json:"url,omitempty"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// AgencyResponse is the public response returned by get_agency.
type AgencyResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// StopResponse is returned by stop lookup and stop-list tools.
type StopResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Code      string   `json:"code"`
	Direction string   `json:"direction,omitempty"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	RouteIDs  []string `json:"route_ids,omitempty"`
}

type StopIDsResponse []string

// RouteResponse is returned by route lookup and route-list tools.
type RouteResponse struct {
	ID        string `json:"id"`
	ShortName string `json:"short_name"`
	LongName  string `json:"long_name,omitempty"`
	AgencyID  string `json:"agency_id"`
	Color     string `json:"color,omitempty"`
}

type RouteIDsResponse []string

// ArrivalResponse is returned by arrival and departure tools.
type ArrivalResponse struct {
	TripID             string  `json:"trip_id"`
	ServiceDate        int64   `json:"service_date,omitempty"`
	RouteID            string  `json:"route_id"`
	RouteName          string  `json:"route_name"`
	Headsign           string  `json:"headsign"`
	Predicted          bool    `json:"predicted"`
	ScheduledArrival   string  `json:"scheduled_arrival"`
	PredictedArrival   string  `json:"predicted_arrival,omitempty"`
	ScheduledDeparture string  `json:"scheduled_departure"`
	PredictedDeparture string  `json:"predicted_departure,omitempty"`
	NumberOfStopsAway  *int    `json:"number_of_stops_away,omitempty"`
	DistanceFromStop   float64 `json:"distance_from_stop_meters,omitempty"`
	Status             string  `json:"status"`
	VehicleID          string  `json:"vehicle_id,omitempty"`
	VehicleLat         float64 `json:"vehicle_lat,omitempty"`
	VehicleLon         float64 `json:"vehicle_lon,omitempty"`
	VehicleBearing     float64 `json:"vehicle_bearing,omitempty"`
}

// StopOverviewResponse is returned by get_stop_overview.
type StopOverviewResponse struct {
	StopID   string            `json:"stop_id"`
	StopName string            `json:"stop_name,omitempty"`
	Lat      float64           `json:"lat,omitempty"`
	Lon      float64           `json:"lon,omitempty"`
	Routes   []string          `json:"routes_serving,omitempty"`
	Next     []ArrivalResponse `json:"next_arrivals,omitempty"`
}

// StopScheduleResponse is returned by get_stop_schedule.
type ScheduledTripResponse struct {
	TripID    string `json:"trip_id"`
	Departure string `json:"departure"`
}
type DirectionScheduleResponse struct {
	Headsign string                  `json:"headsign"`
	Trips    []ScheduledTripResponse `json:"trips"`
}
type RouteScheduleResponse struct {
	RouteID    string                      `json:"route_id"`
	Directions []DirectionScheduleResponse `json:"directions"`
}
type StopScheduleResponse struct {
	StopID string                  `json:"stop_id"`
	Date   string                  `json:"date"`
	Routes []RouteScheduleResponse `json:"routes"`
}

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

type TripResponse struct {
	ID          string `json:"id"`
	RouteID     string `json:"route_id"`
	Headsign    string `json:"headsign,omitempty"`
	DirectionID string `json:"direction_id,omitempty"`
	ServiceID   string `json:"service_id,omitempty"`
}
type TripDetailsResponse struct {
	TripID        string  `json:"trip_id"`
	RouteID       string  `json:"route_id,omitempty"`
	ShapeID       string  `json:"shape_id,omitempty"`
	Phase         string  `json:"phase,omitempty"`
	Status        string  `json:"status,omitempty"`
	VehicleID     string  `json:"vehicle_id,omitempty"`
	Lat           float64 `json:"lat,omitempty"`
	Lon           float64 `json:"lon,omitempty"`
	Bearing       float64 `json:"bearing,omitempty"`
	DeviationMins float64 `json:"deviation_mins,omitempty"`
}
type VehicleResponse struct {
	VehicleID     string  `json:"vehicle_id"`
	TripID        string  `json:"trip_id,omitempty"`
	RouteID       string  `json:"route_id,omitempty"`
	Headsign      string  `json:"headsign,omitempty"`
	Phase         string  `json:"phase,omitempty"`
	Lat           float64 `json:"lat,omitempty"`
	Lon           float64 `json:"lon,omitempty"`
	DeviationMins float64 `json:"deviation_mins,omitempty"`
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
type BlockTripResponse struct {
	TripID   string `json:"trip_id"`
	RouteID  string `json:"route_id,omitempty"`
	Headsign string `json:"headsign,omitempty"`
}
type BlockConfigurationResponse struct {
	ActiveServiceIDs []string            `json:"active_service_ids"`
	Trips            []BlockTripResponse `json:"trips"`
}
type BlockResponse struct {
	ID             string                       `json:"id"`
	Configurations []BlockConfigurationResponse `json:"configurations"`
}
type CurrentTimeResponse struct {
	Time string `json:"time"`
}
type MetadataResponse struct {
	StaticGTFSLastUpdated string            `json:"static_gtfs_last_updated,omitempty"`
	RealtimeFeeds         map[string]string `json:"realtime_feeds,omitempty"`
}
