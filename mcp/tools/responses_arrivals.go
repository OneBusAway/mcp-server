package tools

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
