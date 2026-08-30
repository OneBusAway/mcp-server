package tools

// ArrivalResponse is returned by arrival and departure tools.
type ArrivalResponse struct {
	TripID                    string  `json:"trip_id"`
	ServiceDateMS             int64   `json:"service_date_ms,omitempty"`
	RouteID                   string  `json:"route_id"`
	RouteName                 string  `json:"route_name"`
	Headsign                  string  `json:"headsign"`
	Predicted                 bool    `json:"predicted"`
	Timezone                  string  `json:"timezone"`
	ScheduledArrivalMS        int64   `json:"scheduled_arrival_ms,omitempty"`
	ScheduledArrivalDisplay   string  `json:"scheduled_arrival_display,omitempty"`
	PredictedArrivalMS        int64   `json:"predicted_arrival_ms,omitempty"`
	PredictedArrivalDisplay   string  `json:"predicted_arrival_display,omitempty"`
	ScheduledDepartureMS      int64   `json:"scheduled_departure_ms,omitempty"`
	ScheduledDepartureDisplay string  `json:"scheduled_departure_display,omitempty"`
	PredictedDepartureMS      int64   `json:"predicted_departure_ms,omitempty"`
	PredictedDepartureDisplay string  `json:"predicted_departure_display,omitempty"`
	NumberOfStopsAway         *int    `json:"number_of_stops_away,omitempty"`
	DistanceFromStop          float64 `json:"distance_from_stop_meters,omitempty"`
	Status                    string  `json:"status"`
	VehicleID                 string  `json:"vehicle_id,omitempty"`
	VehicleLat                float64 `json:"vehicle_lat,omitempty"`
	VehicleLon                float64 `json:"vehicle_lon,omitempty"`
	VehicleBearing            float64 `json:"vehicle_bearing,omitempty"`
}
