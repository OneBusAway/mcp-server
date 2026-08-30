package tools

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
	TripID           string `json:"trip_id"`
	DepartureMS      int64  `json:"departure_ms"`
	DepartureDisplay string `json:"departure_display"`
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
	StopID      string                  `json:"stop_id"`
	DateMS      int64                   `json:"date_ms,omitempty"`
	DateDisplay string                  `json:"date_display,omitempty"`
	Timezone    string                  `json:"timezone"`
	Routes      []RouteScheduleResponse `json:"routes"`
}
