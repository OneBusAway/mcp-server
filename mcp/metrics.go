package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"oba-mcp/client"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 60}
var responseSizeBuckets = []float64{256, 1024, 4096, 16384, 65536, 131072}

type metricKey struct {
	name      string
	outcome   string
	errorCode string
	cache     string
	status    string
}

type metricHistogram struct {
	count   uint64
	sum     float64
	buckets []uint64
}

type operationalMetrics struct {
	mu sync.Mutex

	toolRequests      map[metricKey]uint64
	toolDurations     map[metricKey]*metricHistogram
	toolResponseSizes map[metricKey]*metricHistogram
	upstreamRequests  map[metricKey]uint64
	upstreamDurations map[metricKey]*metricHistogram
	upstreamSizes     map[metricKey]*metricHistogram
	concurrencyLimits map[string]uint64
	circuitOpen       bool
	toolInFlight      atomic.Int64
}

func newOperationalMetrics() *operationalMetrics {
	return &operationalMetrics{
		toolRequests:      make(map[metricKey]uint64),
		toolDurations:     make(map[metricKey]*metricHistogram),
		toolResponseSizes: make(map[metricKey]*metricHistogram),
		upstreamRequests:  make(map[metricKey]uint64),
		upstreamDurations: make(map[metricKey]*metricHistogram),
		upstreamSizes:     make(map[metricKey]*metricHistogram),
		concurrencyLimits: map[string]uint64{"wait_cancelled": 0},
	}
}

func (m *operationalMetrics) beginTool() {
	m.toolInFlight.Add(1)
}

func (m *operationalMetrics) endTool() {
	m.toolInFlight.Add(-1)
}

func (m *operationalMetrics) observeTool(tool, outcome, errorCode string, duration time.Duration, responseBytes int) {
	key := metricKey{name: tool, outcome: outcome, errorCode: errorCode}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolRequests[key]++
	observeHistogram(m.toolDurations, key, duration.Seconds(), durationBuckets)
	observeHistogram(m.toolResponseSizes, key, float64(responseBytes), responseSizeBuckets)
}

func (m *operationalMetrics) ObserveUpstream(observation client.UpstreamObservation) {
	errorCode := string(observation.ErrorCode)
	status := "cache"
	if observation.StatusCode != 0 {
		status = strconv.Itoa(observation.StatusCode)
	} else if errorCode != "" {
		status = "transport_error"
	}
	key := metricKey{name: observation.Operation, cache: observation.Cache, errorCode: errorCode, status: status}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upstreamRequests[key]++
	observeHistogram(m.upstreamDurations, key, observation.Duration.Seconds(), durationBuckets)
	observeHistogram(m.upstreamSizes, key, float64(observation.Bytes), responseSizeBuckets)
}

func (m *operationalMetrics) ObserveCircuit(state string) {
	m.mu.Lock()
	m.circuitOpen = state == "open"
	m.mu.Unlock()
}

func (m *operationalMetrics) ObserveConcurrencyLimit(outcome string) {
	m.mu.Lock()
	m.concurrencyLimits[outcome]++
	m.mu.Unlock()
}

func observeHistogram(histograms map[metricKey]*metricHistogram, key metricKey, value float64, bounds []float64) {
	histogram := histograms[key]
	if histogram == nil {
		histogram = &metricHistogram{buckets: make([]uint64, len(bounds))}
		histograms[key] = histogram
	}
	histogram.count++
	histogram.sum += value
	for index, bound := range bounds {
		if value <= bound {
			histogram.buckets[index]++
		}
	}
}

func (m *operationalMetrics) handler(w http.ResponseWriter, r *http.Request) {
	if !healthMethodAllowed(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(m.render()))
}

func (m *operationalMetrics) render() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var output strings.Builder
	output.WriteString("# HELP oba_mcp_tool_in_flight Current MCP tool calls.\n")
	output.WriteString("# TYPE oba_mcp_tool_in_flight gauge\n")
	fmt.Fprintf(&output, "oba_mcp_tool_in_flight %d\n", m.toolInFlight.Load())
	output.WriteString("# HELP oba_mcp_tool_requests_total Completed MCP tool calls.\n")
	output.WriteString("# TYPE oba_mcp_tool_requests_total counter\n")
	writeCounters(&output, "oba_mcp_tool_requests_total", m.toolRequests, toolLabels)
	output.WriteString("# HELP oba_mcp_tool_duration_seconds MCP tool call latency.\n")
	output.WriteString("# TYPE oba_mcp_tool_duration_seconds histogram\n")
	writeHistograms(&output, "oba_mcp_tool_duration_seconds", m.toolDurations, durationBuckets, toolLabels)
	output.WriteString("# HELP oba_mcp_tool_response_bytes MCP structured response size.\n")
	output.WriteString("# TYPE oba_mcp_tool_response_bytes histogram\n")
	writeHistograms(&output, "oba_mcp_tool_response_bytes", m.toolResponseSizes, responseSizeBuckets, toolLabels)
	output.WriteString("# HELP oba_mcp_upstream_requests_total OBA request attempts and cache reads.\n")
	output.WriteString("# TYPE oba_mcp_upstream_requests_total counter\n")
	writeCounters(&output, "oba_mcp_upstream_requests_total", m.upstreamRequests, upstreamLabels)
	output.WriteString("# HELP oba_mcp_upstream_duration_seconds OBA request attempt latency.\n")
	output.WriteString("# TYPE oba_mcp_upstream_duration_seconds histogram\n")
	writeHistograms(&output, "oba_mcp_upstream_duration_seconds", m.upstreamDurations, durationBuckets, upstreamLabels)
	output.WriteString("# HELP oba_mcp_upstream_response_bytes OBA response size, including cache reads.\n")
	output.WriteString("# TYPE oba_mcp_upstream_response_bytes histogram\n")
	writeHistograms(&output, "oba_mcp_upstream_response_bytes", m.upstreamSizes, responseSizeBuckets, upstreamLabels)
	output.WriteString("# HELP oba_mcp_upstream_concurrency_limit_total Calls that could not acquire upstream concurrency before cancellation.\n")
	output.WriteString("# TYPE oba_mcp_upstream_concurrency_limit_total counter\n")
	writeStringCounters(&output, "oba_mcp_upstream_concurrency_limit_total", "outcome", m.concurrencyLimits)
	output.WriteString("# HELP oba_mcp_circuit_open Whether the OBA circuit breaker is open.\n")
	output.WriteString("# TYPE oba_mcp_circuit_open gauge\n")
	if m.circuitOpen {
		output.WriteString("oba_mcp_circuit_open 1\n")
	} else {
		output.WriteString("oba_mcp_circuit_open 0\n")
	}
	return output.String()
}

type labelFunc func(metricKey) string

func toolLabels(key metricKey) string {
	return labels("tool", key.name, "outcome", key.outcome, "error_code", key.errorCode)
}

func upstreamLabels(key metricKey) string {
	return labels("operation", key.name, "cache", key.cache, "status", key.status, "error_code", key.errorCode)
}

func labels(values ...string) string {
	var output strings.Builder
	output.WriteByte('{')
	for index := 0; index < len(values); index += 2 {
		if index > 0 {
			output.WriteByte(',')
		}
		output.WriteString(values[index])
		output.WriteByte('=')
		output.WriteString(strconv.Quote(values[index+1]))
	}
	output.WriteByte('}')
	return output.String()
}

func sortedMetricKeys[T any](values map[metricKey]T) []metricKey {
	keys := make([]metricKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return metricKeyLess(keys[i], keys[j])
	})
	return keys
}

func metricKeyLess(left, right metricKey) bool {
	if left.name != right.name {
		return left.name < right.name
	}
	if left.outcome != right.outcome {
		return left.outcome < right.outcome
	}
	if left.errorCode != right.errorCode {
		return left.errorCode < right.errorCode
	}
	if left.cache != right.cache {
		return left.cache < right.cache
	}
	if left.status != right.status {
		return left.status < right.status
	}
	return false
}

func writeCounters(output *strings.Builder, name string, counters map[metricKey]uint64, labeler labelFunc) {
	for _, key := range sortedMetricKeys(counters) {
		fmt.Fprintf(output, "%s%s %d\n", name, labeler(key), counters[key])
	}
}

func writeStringCounters(output *strings.Builder, name, labelName string, counters map[string]uint64) {
	keys := make([]string, 0, len(counters))
	for key := range counters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(output, "%s%s %d\n", name, labels(labelName, key), counters[key])
	}
}

func writeHistograms(output *strings.Builder, name string, histograms map[metricKey]*metricHistogram, bounds []float64, labeler labelFunc) {
	for _, key := range sortedMetricKeys(histograms) {
		histogram := histograms[key]
		baseLabels := strings.TrimSuffix(labeler(key), "}")
		for index, bound := range bounds {
			fmt.Fprintf(output, "%s_bucket%s,le=%q} %d\n", name, baseLabels, strconv.FormatFloat(bound, 'g', -1, 64), histogram.buckets[index])
		}
		fmt.Fprintf(output, "%s_bucket%s,le=\"+Inf\"} %d\n", name, baseLabels, histogram.count)
		fmt.Fprintf(output, "%s_sum%s %.9g\n", name, labeler(key), histogram.sum)
		fmt.Fprintf(output, "%s_count%s %d\n", name, labeler(key), histogram.count)
	}
}
