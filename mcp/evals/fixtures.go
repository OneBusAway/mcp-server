package evals

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"oba-mcp/internal/obafixture"
)

// FixtureServer is an isolated, deterministic OBA endpoint for one eval case.
type FixtureServer struct {
	URL   string
	close func()
}

// Close releases the fixture server.
func (s *FixtureServer) Close() {
	if s != nil && s.close != nil {
		s.close()
	}
}

// StartFixture starts the OBA behavior declared by testCase. Both deterministic
// handler tests and live model evals use this path so they exercise identical
// upstream responses.
func StartFixture(testCase Case) (*FixtureServer, error) {
	if testCase.Type == "upstream_failure" {
		return startFailingFixture(testCase.ExpectedErrorCode, testCase.Fixture), nil
	}

	responses, err := fixtureResponses(testCase.Fixture)
	if err != nil {
		return nil, err
	}
	fixture := obafixture.New(responses)
	return &FixtureServer{URL: fixture.URL, close: fixture.Close}, nil
}

func startFailingFixture(expectedErrorCode, fixtureName string) *FixtureServer {
	if fixtureName == "malformed_body" {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":`))
		}))
		return &FixtureServer{URL: upstream.URL, close: upstream.Close}
	}

	status, body := statusForExpectedErrorCode(expectedErrorCode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return &FixtureServer{URL: upstream.URL, close: upstream.Close}
}

func fixtureResponses(fixtureName string) (map[string]obafixture.Response, error) {
	responses := successFixtureResponses()
	entry := func(body string) obafixture.Response {
		return obafixture.Response{Body: `{"code":200,"data":{"entry":` + body + `}}`}
	}
	list := func(body string) obafixture.Response {
		return obafixture.Response{Body: `{"code":200,"data":{"list":` + body + `}}`}
	}
	switch fixtureName {
	case "ambiguous_stops":
		responses["/api/where/search/stop.json"] = list(`[
			{"id":"test_1001","name":"Main Street at First Avenue","lat":27.9,"lon":-82.4},
			{"id":"test_1002","name":"Main Street at Second Avenue","lat":27.91,"lon":-82.41}
		]`)
	case "stale_arrival":
		responses["/api/where/arrivals-and-departures-for-stop/test_1013.json"] = entry(`{
			"stopId":"test_1013",
			"arrivalsAndDepartures":[{
				"tripId":"test_trip","routeId":"test_10","routeShortName":"10",
				"predicted":true,"predictedArrivalTime":1770000000000,"scheduledArrivalTime":1770000000000
			}]
		}`)
	case "stale_metadata":
		responses["/api/v2/metadata.json"] = obafixture.Response{Body: `{
			"staticGtfsLastUpdated":"2026-01-01T00:00:00Z",
			"realtimeFeeds":{"trip_updates":"2026-01-01T00:00:00Z"}
		}`}
	case "empty_arrivals":
		responses["/api/where/arrivals-and-departures-for-stop/test_1013.json"] = entry(`{"stopId":"test_1013","arrivalsAndDepartures":[]}`)
	case "", "malformed_body":
	default:
		return nil, fmt.Errorf("unknown evaluation fixture %q", fixtureName)
	}
	return responses, nil
}

func statusForExpectedErrorCode(code string) (int, string) {
	switch code {
	case "UPSTREAM_RATE_LIMITED":
		return http.StatusTooManyRequests, `{"code":429,"text":"rate limited"}`
	case "UPSTREAM_BAD_RESPONSE":
		return http.StatusBadRequest, `{"code":400,"text":"bad request"}`
	default:
		return http.StatusServiceUnavailable, `{"code":503,"text":"unavailable"}`
	}
}

func successFixtureResponses() map[string]obafixture.Response {
	entry := func(body string) obafixture.Response {
		return obafixture.Response{Body: `{"code":200,"data":{"entry":` + body + `}}`}
	}
	list := func(body string) obafixture.Response {
		return obafixture.Response{Body: `{"code":200,"data":{"list":` + body + `}}`}
	}
	agencyBody := `{"id":"test","name":"Test Agency","url":"https://example.com","phone":"","timezone":"America/Los_Angeles"}`
	stopBody := func(id string) string {
		return `{"id":"` + id + `","name":"Fixture Stop","code":"1013","direction":"North","lat":27.9488,"lon":-82.4582,"routeIds":["test_10"]}`
	}
	return map[string]obafixture.Response{
		"/api/where/agencies-with-coverage.json":                     list(`[{"agencyId":"test","lat":27.9488,"lon":-82.4582}]`),
		"/api/where/agency/test.json":                                entry(agencyBody),
		"/api/where/stop-ids-for-agency/test.json":                   list(`["test_1013"]`),
		"/api/where/stop/test_1013.json":                             entry(stopBody("test_1013")),
		"/api/where/stop/test_1013111.json":                          entry(stopBody("test_1013111")),
		"/api/where/search/stop.json":                                list(`[]`),
		"/api/where/stops-for-location.json":                         list(`[]`),
		"/api/where/stops-for-agency/test.json":                      list(`[]`),
		"/api/where/schedule-for-stop/test_1013.json":                entry(`{"stopId":"test_1013","stopRouteSchedules":[]}`),
		"/api/where/current-time.json":                               entry(`{"time":1770000000000,"readableTime":"2026-01-01T00:00:00Z"}`),
		"/api/v2/metadata.json":                                      {Body: `{"realtimeFeeds":{}}`},
		"/api/where/arrival-and-departure-for-stop/test_1013.json":   entry(`{"stopId":"test_1013","tripId":"test_trip","routeId":"test_10","routeShortName":"10","predicted":false,"scheduledArrivalTime":1770000000000}`),
		"/api/where/arrivals-and-departures-for-stop/test_1013.json": entry(`{"stopId":"test_1013","arrivalsAndDepartures":[]}`),
		"/api/where/arrivals-and-departures-for-location.json":       entry(`{"arrivalsAndDepartures":[]}`),
		"/api/where/route-ids-for-agency/test.json":                  list(`["test_10"]`),
		"/api/where/shape/test_shape.json":                           entry(`{"length":0,"levels":"","points":"encoded"}`),
		"/api/where/route/test_10.json":                              entry(`{"id":"test_10","shortName":"10","longName":"Test Route","agencyId":"test"}`),
		"/api/where/search/route.json":                               list(`[]`),
		"/api/where/routes-for-agency/test.json":                     list(`[]`),
		"/api/where/routes-for-location.json":                        list(`[]`),
		"/api/where/stops-for-route/test_10.json":                    entry(`{"routeId":"test_10","stopIds":[],"stopGroupings":[]}`),
		"/api/where/trips-for-route/test_10.json":                    list(`[]`),
		"/api/where/schedule-for-route/test_10.json":                 entry(`{"routeId":"test_10","scheduleDate":0,"trips":[],"stopTripGroupings":[]}`),
		"/api/where/block/test_block.json":                           entry(`{"id":"test_block","configurations":[]}`),
		"/api/where/trip/test_trip.json":                             entry(`{"id":"test_trip","routeId":"test_10"}`),
		"/api/where/trip-details/test_trip.json":                     entry(`{"tripId":"test_trip","serviceDate":0,"schedule":{"stopTimes":[]}}`),
		"/api/where/trip-for-vehicle/test_bus.json":                  entry(`{"tripId":"test_trip","serviceDate":0,"schedule":{"stopTimes":[]}}`),
		"/api/where/vehicles-for-agency/test.json":                   list(`[]`),
		"/api/where/trips-for-location.json":                         list(`[]`),
	}
}
