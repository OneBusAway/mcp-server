package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"oba-mcp/client"
	"oba-mcp/internal/obafixture"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// fixtureHandler wires a Handler to a deterministic OBA fixture and returns a
// tear-down so tests do not have to remember cleanup. The returned handler is
// safe to invoke concurrently; the fixture records every request path and
// query so tests can also assert on outgoing calls.
func fixtureHandler(t *testing.T, responses map[string]string) (*Handler, *obafixture.Server) {
	t.Helper()
	converted := make(map[string]obafixture.Response, len(responses))
	for path, body := range responses {
		converted[path] = obafixture.Response{Body: body}
	}
	upstream := obafixture.New(converted)
	t.Cleanup(upstream.Close)
	return &Handler{client: client.New(upstream.URL, "fixture-api-key", nil, nil)}, upstream
}

// invokeHandler calls the given tool handler with typed arguments and asserts
// the returned MCP result is well-formed. Test callers still need to unwrap
// the structured content into their expected type.
func invokeHandler(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := handler(context.Background(), toolRequest(arguments))
	if err != nil {
		t.Fatalf("handler returned protocol error: %v", err)
	}
	if result == nil {
		t.Fatalf("handler returned nil result")
	}
	if result.IsError {
		var envelope ErrorEnvelope
		if raw, ok := result.StructuredContent.(ErrorEnvelope); ok {
			envelope = raw
		}
		t.Fatalf("handler returned tool error: code=%q message=%q", envelope.Code, envelope.Message)
	}
	return result
}

// dataAs pulls the tool's structured Data out of the SuccessEnvelope[any] and
// asserts it is of the expected concrete type. Handlers set Data to a typed
// value via toResult; this helper puts the assertion boilerplate in one
// place so each per-tool test remains focused on field-level checks.
func dataAs[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	envelope, ok := result.StructuredContent.(SuccessEnvelope[any])
	if !ok {
		var zero T
		t.Fatalf("structured content type = %T, want SuccessEnvelope[any]", result.StructuredContent)
		return zero
	}
	value, ok := envelope.Data.(T)
	if !ok {
		var zero T
		t.Fatalf("envelope data type = %T, want %T", envelope.Data, zero)
		return zero
	}
	return value
}

// meta returns the SuccessEnvelope meta fields (cache state, truncation) so
// tests can assert the boundary contract without repeating the assertion.
func meta(t *testing.T, result *mcp.CallToolResult) ResponseMeta {
	t.Helper()
	envelope, ok := result.StructuredContent.(SuccessEnvelope[any])
	if !ok {
		t.Fatalf("structured content type = %T, want SuccessEnvelope[any]", result.StructuredContent)
	}
	return envelope.Meta
}

// assertRequestPath asserts that the fixture recorded a request for path with
// at least the given query keys/values. Extra keys (e.g. the API key) are
// permitted so tests can focus on tool-specific parameters.
func assertRequestPath(t *testing.T, upstream *obafixture.Server, path string, wantQuery map[string]string) {
	t.Helper()
	for _, request := range upstream.Requests() {
		if request.Path != path {
			continue
		}
		matched := true
		for key, want := range wantQuery {
			if got := request.Query.Get(key); got != want {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("no fixture request matched path=%q query=%v; recorded=%#v", path, wantQuery, upstream.Requests())
}

// tzAwareAgency is the fixture body used whenever a handler needs to resolve
// an agency timezone via client.TimezoneFor. Using a single constant keeps
// timestamp-preservation assertions stable across the suite.
const tzAwareAgency = `{"code":200,"data":{"entry":{"id":"test","name":"Test Agency","timezone":"America/Los_Angeles"}}}`

// envelopeEntry wraps a JSON entry body in the standard OBA success envelope.
func envelopeEntry(body string) string {
	return `{"code":200,"data":{"entry":` + body + `}}`
}

// envelopeList wraps a JSON list body in the standard OBA success envelope.
func envelopeList(body string) string {
	return `{"code":200,"data":{"list":` + body + `}}`
}

// envelopeListWithReferences wraps a list body with references (agencies,
// stops, trips) that many OBA endpoints resolve at the client. Passing an
// empty references object keeps the DTO decoders happy without needing extra
// stub data.
func envelopeListWithReferences(body, references string) string {
	return `{"code":200,"data":{"list":` + body + `,"references":` + references + `}}`
}

// jsonNonEmpty is a defensive helper: it fails the test if the given raw JSON
// is empty or invalid. Used by tests that build the fixture response inline
// to catch typos before they surface as decoding errors deeper in the stack.
func jsonNonEmpty(t *testing.T, raw string) {
	t.Helper()
	if raw == "" || !json.Valid([]byte(raw)) {
		t.Fatalf("fixture body is empty or not valid JSON: %q", raw)
	}
}

// TestEveryRegisteredToolHasAFixtureContract fails when a newly registered
// tool lacks a fixture-driven contract test, or when contractCoveredTools
// references a tool that is no longer registered. It closes the "add tool,
// forget to add a test" loophole that structural output-schema tests miss.
//
// Because the map values are typed func(*testing.T) references, deleting or
// renaming a test function is a compile error — not a runtime skip. This
// test additionally verifies that every entry actually points at a real
// function via its runtime name, so a nil literal (or a mistakenly reused
// reference) is also caught.
func TestEveryRegisteredToolHasAFixtureContract(t *testing.T) {
	obaClient := client.New("http://example.invalid", "test-key", nil, nil)
	mcpServer := server.NewMCPServer("test", "test")
	RegisterAll(mcpServer, obaClient)

	registered := make(map[string]struct{}, len(mcpServer.ListTools()))
	for name := range mcpServer.ListTools() {
		registered[name] = struct{}{}
	}

	var missing, stale []string
	seenTestFunctions := make(map[uintptr]string, len(contractCoveredTools))
	for name := range registered {
		if _, ok := contractCoveredTools[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name, testFn := range contractCoveredTools {
		if _, ok := registered[name]; !ok {
			stale = append(stale, name)
			continue
		}
		if testFn == nil {
			t.Errorf("contractCoveredTools[%q] is a nil test reference", name)
			continue
		}
		pointer := reflect.ValueOf(testFn).Pointer()
		if otherTool, duplicate := seenTestFunctions[pointer]; duplicate {
			t.Errorf("contractCoveredTools[%q] reuses the contract test registered for %q", name, otherTool)
			continue
		}
		seenTestFunctions[pointer] = name
		fnName := runtime.FuncForPC(pointer).Name()
		if !strings.Contains(fnName, ".Test") {
			t.Errorf("contractCoveredTools[%q] points at %q, which does not look like a Test* function",
				name, fnName)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("registered tools without a fixture contract test: %v — add an entry to contractCoveredTools and a Test<Name>Contract function", missing)
	}
	if len(stale) > 0 {
		t.Errorf("contractCoveredTools names unregistered tools: %v — remove the stale entry", stale)
	}
}

// contractCoveredTools maps every registered tool to its fixture-backed
// contract test. The value is a live function reference, not a name string,
// so the compiler enforces existence: deleting or renaming a test breaks the
// build. TestEveryRegisteredToolHasAFixtureContract then cross-checks the
// map keys against the actual server tool list, catching stale or missing
// tool names.
var contractCoveredTools = map[string]func(*testing.T){
	"get_agencies":                       TestGetAgenciesContract,
	"get_agency":                         TestGetAgencyContract,
	"get_stop":                           TestGetStopContract,
	"search_stops":                       TestSearchStopsContract,
	"find_stops_near_location":           TestFindStopsNearLocationContract,
	"get_stops_for_agency":               TestGetStopsForAgencyContract,
	"get_stop_schedule":                  TestGetStopScheduleContract,
	"get_stop_ids_for_agency":            TestGetStopIDsForAgencyContract,
	"get_arrival_and_departure_for_stop": TestGetArrivalAndDepartureForStopContract,
	"get_arrivals_for_stop":              TestArrivalsHandlerTransformsFixedOBADTOs,
	"get_arrivals_for_location":          TestGetArrivalsForLocationContract,
	"get_route":                          TestGetRouteContract,
	"search_routes":                      TestSearchRoutesContract,
	"get_routes_for_agency":              TestGetRoutesForAgencyContract,
	"get_routes_for_location":            TestGetRoutesForLocationContract,
	"get_stops_for_route":                TestGetStopsForRouteContract,
	"get_trips_for_route":                TestGetTripsForRouteContract,
	"get_schedule_for_route":             TestGetScheduleForRouteContract,
	"get_route_ids_for_agency":           TestGetRouteIDsForAgencyContract,
	"get_shape":                          TestGetShapeContract,
	"get_trip":                           TestGetTripContract,
	"get_trip_details":                   TestGetTripDetailsContract,
	"get_trip_for_vehicle":               TestGetTripForVehicleContract,
	"get_vehicles_for_agency":            TestGetVehiclesForAgencyContract,
	"get_trips_for_location":             TestGetTripsForLocationContract,
	"get_block":                          TestGetBlockContract,
	"get_current_time":                   TestGetCurrentTimeContract,
	"get_metadata":                       TestGetMetadataContract,
	"get_stop_overview":                  TestGetStopOverviewContract,
}
