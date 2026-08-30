package tools

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
