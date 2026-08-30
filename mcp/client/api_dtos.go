// Package client contains typed OneBusAway REST API data-transfer objects.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// ListResponse and EntryResponse mirror Maglev's typed /api/where envelope.
// They intentionally contain only fields used by oba-mcp.
type ListResponse[T any] struct {
	Code int    `json:"code"`
	Text string `json:"text"`
	Data struct {
		List       []T        `json:"list"`
		References References `json:"references"`
	} `json:"data"`
}

type EntryResponse[T any] struct {
	Code int    `json:"code"`
	Text string `json:"text"`
	Data struct {
		Entry      T          `json:"entry"`
		References References `json:"references"`
	} `json:"data"`
}

type envelopeStatus struct {
	Code *int   `json:"code"`
	Text string `json:"text"`
}

// References is the subset of Maglev's references envelope used to enrich
// compact MCP responses.
type References struct {
	Agencies []Agency `json:"agencies"`
	Stops    []Stop   `json:"stops"`
	Trips    []Trip   `json:"trips"`
}

type AgencyCoverage struct {
	AgencyID string  `json:"agencyId"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}
type Agency struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Phone    string `json:"phone"`
	Timezone string `json:"timezone"`
}

type Stop struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Code      string   `json:"code"`
	Direction string   `json:"direction"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	RouteIDs  []string `json:"routeIds"`
}

type Route struct {
	ID        string `json:"id"`
	ShortName string `json:"shortName"`
	LongName  string `json:"longName"`
	AgencyID  string `json:"agencyId"`
	Color     string `json:"color"`
}

type Position struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type TripStatus struct {
	ActiveTripID      string    `json:"activeTripId"`
	Phase             string    `json:"phase"`
	Status            string    `json:"status"`
	VehicleID         string    `json:"vehicleId"`
	ScheduleDeviation float64   `json:"scheduleDeviation"`
	Orientation       float64   `json:"orientation"`
	Position          *Position `json:"position"`
	ActiveTrip        *Trip     `json:"activeTrip"`
}

type Trip struct {
	ID           string `json:"id"`
	RouteID      string `json:"routeId"`
	ShapeID      string `json:"shapeId"`
	TripHeadsign string `json:"tripHeadsign"`
	DirectionID  string `json:"directionId"`
	ServiceID    string `json:"serviceId"`
	BlockID      string `json:"blockId"`
}

type TripDetails struct {
	TripID string      `json:"tripId"`
	Status *TripStatus `json:"status"`
	Trip   *Trip       `json:"trip"`
}

type TripForRoute struct {
	TripID string      `json:"tripId"`
	Status *TripStatus `json:"status"`
	Trip   *Trip       `json:"trip"`
}

type VehicleStatus struct {
	VehicleID  string      `json:"vehicleId"`
	TripID     string      `json:"tripId"`
	Phase      string      `json:"phase"`
	Location   *Position   `json:"location"`
	TripStatus *TripStatus `json:"tripStatus"`
}

type ArrivalAndDeparture struct {
	TripID                 string      `json:"tripId"`
	ServiceDate            int64       `json:"serviceDate"`
	RouteID                string      `json:"routeId"`
	RouteShortName         string      `json:"routeShortName"`
	TripHeadsign           string      `json:"tripHeadsign"`
	Status                 string      `json:"status"`
	VehicleID              string      `json:"vehicleId"`
	Predicted              bool        `json:"predicted"`
	DistanceFromStop       float64     `json:"distanceFromStop"`
	NumberOfStopsAway      int         `json:"numberOfStopsAway"`
	ScheduledArrivalTime   int64       `json:"scheduledArrivalTime"`
	ScheduledDepartureTime int64       `json:"scheduledDepartureTime"`
	PredictedArrivalTime   int64       `json:"predictedArrivalTime"`
	PredictedDepartureTime int64       `json:"predictedDepartureTime"`
	TripStatus             *TripStatus `json:"tripStatus"`
}

type ArrivalsAndDepartures struct {
	StopID                string                `json:"stopId"`
	ArrivalsAndDepartures []ArrivalAndDeparture `json:"arrivalsAndDepartures"`
}

type Polyline struct {
	Points string  `json:"points"`
	Length float64 `json:"length"`
}

type StopGroupName struct {
	Name string `json:"name"`
}
type StopGroup struct {
	Name    StopGroupName `json:"name"`
	StopIDs []string      `json:"stopIds"`
}
type StopGrouping struct {
	StopGroups []StopGroup `json:"stopGroups"`
}
type StopsForRoute struct {
	RouteID       string         `json:"routeId"`
	StopGroupings []StopGrouping `json:"stopGroupings"`
	Polylines     []Polyline     `json:"polylines"`
}

type StopTripGrouping struct {
	DirectionID   string   `json:"directionId"`
	TripHeadsigns []string `json:"tripHeadsigns"`
	TripIDs       []string `json:"tripIds"`
	StopIDs       []string `json:"stopIds"`
}
type ScheduleForRoute struct {
	RouteID           string             `json:"routeId"`
	ScheduleDate      int64              `json:"scheduleDate"`
	StopTripGroupings []StopTripGrouping `json:"stopTripGroupings"`
}

type ScheduleStopTime struct {
	TripID        string `json:"tripId"`
	DepartureTime int64  `json:"departureTime"`
}
type StopRouteDirectionSchedule struct {
	TripHeadsign      string             `json:"tripHeadsign"`
	ScheduleStopTimes []ScheduleStopTime `json:"scheduleStopTimes"`
}
type StopRouteSchedule struct {
	RouteID                     string                       `json:"routeId"`
	StopRouteDirectionSchedules []StopRouteDirectionSchedule `json:"stopRouteDirectionSchedules"`
}
type ScheduleForStop struct {
	Date               int64               `json:"date"`
	StopID             string              `json:"stopId"`
	StopRouteSchedules []StopRouteSchedule `json:"stopRouteSchedules"`
}

type Shape struct {
	Points string  `json:"points"`
	Length float64 `json:"length"`
}
type BlockTrip struct {
	TripID string `json:"tripId"`
	Trip   *Trip  `json:"trip"`
}
type BlockConfiguration struct {
	ActiveServiceIDs []string    `json:"activeServiceIds"`
	Trips            []BlockTrip `json:"trips"`
}
type Block struct {
	ID             string               `json:"id"`
	Configurations []BlockConfiguration `json:"configurations"`
}
type CurrentTime struct {
	Time         int64  `json:"time"`
	ReadableTime string `json:"readableTime"`
}
type Metadata struct {
	StaticGTFSLastUpdated *time.Time           `json:"staticGtfsLastUpdated"`
	RealtimeFeeds         map[string]time.Time `json:"realtimeFeeds"`
}

func (c *OBAClient) GetAgencies(ctx context.Context) (ListResponse[AgencyCoverage], error) {
	var response ListResponse[AgencyCoverage]
	err := c.getTyped(ctx, "/api/where/agencies-with-coverage.json", nil, &response)
	return response, err
}

func (c *OBAClient) GetAgency(ctx context.Context, agencyID string) (EntryResponse[Agency], error) {
	var response EntryResponse[Agency]
	err := c.getTyped(ctx, "/api/where/agency/"+url.PathEscape(agencyID)+".json", nil, &response)
	return response, err
}

func (c *OBAClient) GetStop(ctx context.Context, stopID string) (EntryResponse[Stop], error) {
	var response EntryResponse[Stop]
	err := c.getTyped(ctx, "/api/where/stop/"+url.PathEscape(stopID)+".json", nil, &response)
	return response, err
}

func (c *OBAClient) SearchStops(ctx context.Context, params url.Values) (ListResponse[Stop], error) {
	var response ListResponse[Stop]
	err := c.getTyped(ctx, "/api/where/search/stop.json", params, &response)
	return response, err
}
func (c *OBAClient) StopsForLocation(ctx context.Context, params url.Values) (ListResponse[Stop], error) {
	var response ListResponse[Stop]
	err := c.getTyped(ctx, "/api/where/stops-for-location.json", params, &response)
	return response, err
}
func (c *OBAClient) StopsForAgency(ctx context.Context, agencyID string) (ListResponse[Stop], error) {
	var response ListResponse[Stop]
	err := c.getTyped(ctx, "/api/where/stops-for-agency/"+url.PathEscape(agencyID)+".json", nil, &response)
	return response, err
}
func (c *OBAClient) StopIDsForAgency(ctx context.Context, agencyID string) (ListResponse[string], error) {
	var response ListResponse[string]
	err := c.getTyped(ctx, "/api/where/stop-ids-for-agency/"+url.PathEscape(agencyID)+".json", nil, &response)
	return response, err
}

func getList[T any](ctx context.Context, c *OBAClient, path string, params url.Values) (ListResponse[T], error) {
	var response ListResponse[T]
	err := c.getTyped(ctx, path, params, &response)
	return response, err
}
func getEntry[T any](ctx context.Context, c *OBAClient, path string, params url.Values) (EntryResponse[T], error) {
	var response EntryResponse[T]
	err := c.getTyped(ctx, path, params, &response)
	return response, err
}

func (c *OBAClient) GetRoute(ctx context.Context, id string) (EntryResponse[Route], error) {
	return getEntry[Route](ctx, c, "/api/where/route/"+url.PathEscape(id)+".json", nil)
}
func (c *OBAClient) SearchRoutes(ctx context.Context, params url.Values) (ListResponse[Route], error) {
	return getList[Route](ctx, c, "/api/where/search/route.json", params)
}
func (c *OBAClient) RoutesForAgency(ctx context.Context, id string) (ListResponse[Route], error) {
	return getList[Route](ctx, c, "/api/where/routes-for-agency/"+url.PathEscape(id)+".json", nil)
}
func (c *OBAClient) RoutesForLocation(ctx context.Context, params url.Values) (ListResponse[Route], error) {
	return getList[Route](ctx, c, "/api/where/routes-for-location.json", params)
}
func (c *OBAClient) StopsForRoute(ctx context.Context, id string) (EntryResponse[StopsForRoute], error) {
	return getEntry[StopsForRoute](ctx, c, "/api/where/stops-for-route/"+url.PathEscape(id)+".json", nil)
}
func (c *OBAClient) TripsForRoute(ctx context.Context, id string, params url.Values) (ListResponse[TripForRoute], error) {
	return getList[TripForRoute](ctx, c, "/api/where/trips-for-route/"+url.PathEscape(id)+".json", params)
}
func (c *OBAClient) ScheduleForRoute(ctx context.Context, id string, params url.Values) (EntryResponse[ScheduleForRoute], error) {
	return getEntry[ScheduleForRoute](ctx, c, "/api/where/schedule-for-route/"+url.PathEscape(id)+".json", params)
}
func (c *OBAClient) RouteIDsForAgency(ctx context.Context, id string) (ListResponse[string], error) {
	return getList[string](ctx, c, "/api/where/route-ids-for-agency/"+url.PathEscape(id)+".json", nil)
}
func (c *OBAClient) GetShape(ctx context.Context, id string) (EntryResponse[Shape], error) {
	return getEntry[Shape](ctx, c, "/api/where/shape/"+url.PathEscape(id)+".json", nil)
}
func (c *OBAClient) ScheduleForStop(ctx context.Context, id string, params url.Values) (EntryResponse[ScheduleForStop], error) {
	return getEntry[ScheduleForStop](ctx, c, "/api/where/schedule-for-stop/"+url.PathEscape(id)+".json", params)
}
func (c *OBAClient) ArrivalsForStop(ctx context.Context, id string, params url.Values) (EntryResponse[ArrivalsAndDepartures], error) {
	return getEntry[ArrivalsAndDepartures](ctx, c, "/api/where/arrivals-and-departures-for-stop/"+url.PathEscape(id)+".json", params)
}
func (c *OBAClient) ArrivalForStop(ctx context.Context, id string, params url.Values) (EntryResponse[ArrivalAndDeparture], error) {
	return getEntry[ArrivalAndDeparture](ctx, c, "/api/where/arrival-and-departure-for-stop/"+url.PathEscape(id)+".json", params)
}
func (c *OBAClient) ArrivalsForLocation(ctx context.Context, params url.Values) (EntryResponse[ArrivalsAndDepartures], error) {
	return getEntry[ArrivalsAndDepartures](ctx, c, "/api/where/arrivals-and-departures-for-location.json", params)
}
func (c *OBAClient) GetTrip(ctx context.Context, id string) (EntryResponse[Trip], error) {
	return getEntry[Trip](ctx, c, "/api/where/trip/"+url.PathEscape(id)+".json", nil)
}
func (c *OBAClient) TripDetails(ctx context.Context, id string, params url.Values) (EntryResponse[TripDetails], error) {
	return getEntry[TripDetails](ctx, c, "/api/where/trip-details/"+url.PathEscape(id)+".json", params)
}
func (c *OBAClient) TripForVehicle(ctx context.Context, id string, params url.Values) (EntryResponse[TripDetails], error) {
	return getEntry[TripDetails](ctx, c, "/api/where/trip-for-vehicle/"+url.PathEscape(id)+".json", params)
}
func (c *OBAClient) VehiclesForAgency(ctx context.Context, id string) (ListResponse[VehicleStatus], error) {
	return getList[VehicleStatus](ctx, c, "/api/where/vehicles-for-agency/"+url.PathEscape(id)+".json", nil)
}
func (c *OBAClient) TripsForLocation(ctx context.Context, params url.Values) (ListResponse[TripForRoute], error) {
	return getList[TripForRoute](ctx, c, "/api/where/trips-for-location.json", params)
}
func (c *OBAClient) GetBlock(ctx context.Context, id string) (EntryResponse[Block], error) {
	return getEntry[Block](ctx, c, "/api/where/block/"+url.PathEscape(id)+".json", nil)
}
func (c *OBAClient) GetCurrentTime(ctx context.Context) (EntryResponse[CurrentTime], error) {
	return getEntry[CurrentTime](ctx, c, "/api/where/current-time.json", nil)
}
func (c *OBAClient) GetMetadata(ctx context.Context) (Metadata, error) {
	var response Metadata
	err := c.getTyped(ctx, "/api/v2/metadata.json", nil, &response)
	return response, err
}

func (c *OBAClient) getTyped(ctx context.Context, path string, params url.Values, target any) error {
	// Keep all transport, cache, and circuit-breaker behavior centralized
	// in Get. A typed DTO is then decoded at this boundary, so tool handlers
	// never interact with untyped API values.
	response, err := c.Get(ctx, path, params)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(response, target); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}
	var status envelopeStatus
	if err := json.Unmarshal(response, &status); err != nil {
		return fmt.Errorf("parsing response status: %w", err)
	}
	if status.Code != nil && *status.Code != 200 {
		return fmt.Errorf("OBA error (code %d): %s", *status.Code, status.Text)
	}
	return nil
}
