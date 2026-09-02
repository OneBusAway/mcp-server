package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oba-mcp/client"
)

func TestOperationalMetricsRenderBoundedSafeLabels(t *testing.T) {
	metrics := newOperationalMetrics()
	metrics.beginTool()
	metrics.endTool()
	metrics.observeTool("get_stop", "success", "", 25*time.Millisecond, 512)
	metrics.ObserveUpstream(client.UpstreamObservation{
		Operation:  "stop",
		Cache:      "miss",
		StatusCode: http.StatusOK,
		Duration:   20 * time.Millisecond,
		Bytes:      256,
	})
	metrics.ObserveCircuit("open")
	metrics.ObserveConcurrencyLimit("wait_cancelled")

	response := httptest.NewRecorder()
	metrics.handler(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	for _, expected := range []string{
		`oba_mcp_tool_requests_total{tool="get_stop",outcome="success",error_code=""} 1`,
		`oba_mcp_upstream_requests_total{operation="stop",cache="miss",status="200",error_code=""} 1`,
		`oba_mcp_circuit_open 1`,
		`oba_mcp_upstream_concurrency_limit_total{outcome="wait_cancelled"} 1`,
		`le="60"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics omitted %q:\n%s", expected, body)
		}
	}
	for _, forbidden := range []string{"request_id", "caller_hash", "latitude", "longitude", "params"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics exposed forbidden label %q:\n%s", forbidden, body)
		}
	}
}

func TestOperationalMetricsSeedsConcurrencyCancellationSeries(t *testing.T) {
	body := newOperationalMetrics().render()
	if !strings.Contains(body, `oba_mcp_upstream_concurrency_limit_total{outcome="wait_cancelled"} 0`) {
		t.Fatalf("fresh metrics omitted concurrency cancellation series:\n%s", body)
	}
}

func TestMetricKeyLessUsesDeclaredFieldOrder(t *testing.T) {
	tests := []struct {
		name        string
		left, right metricKey
		want        bool
	}{
		{name: "name", left: metricKey{name: "a"}, right: metricKey{name: "b"}, want: true},
		{name: "outcome", left: metricKey{name: "a", outcome: "error"}, right: metricKey{name: "a", outcome: "success"}, want: true},
		{name: "error code", left: metricKey{name: "a", errorCode: "A"}, right: metricKey{name: "a", errorCode: "B"}, want: true},
		{name: "cache", left: metricKey{name: "a", cache: "hit"}, right: metricKey{name: "a", cache: "miss"}, want: true},
		{name: "status", left: metricKey{name: "a", status: "200"}, right: metricKey{name: "a", status: "429"}, want: true},
		{name: "equal", left: metricKey{name: "a"}, right: metricKey{name: "a"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := metricKeyLess(test.left, test.right); got != test.want {
				t.Fatalf("metricKeyLess(%#v, %#v) = %t, want %t", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestOperationalMetricsClassifyUpstreamHTTPFailure(t *testing.T) {
	metrics := newOperationalMetrics()
	metrics.ObserveUpstream(client.UpstreamObservation{
		Operation:  "arrivals-and-departures-for-stop",
		Cache:      "miss",
		ErrorCode:  client.ErrorRateLimited,
		StatusCode: http.StatusTooManyRequests,
	})
	body := metrics.render()
	if !strings.Contains(body, `status="429",error_code="UPSTREAM_RATE_LIMITED"`) {
		t.Fatalf("metrics did not classify upstream status:\n%s", body)
	}
}
