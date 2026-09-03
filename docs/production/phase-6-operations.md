# Phase 6 operations guide

This guide defines the current health, metrics, logging, shutdown, alerting,
and privacy behavior for `oba-mcp`. It supplements the deployment boundary in
[phase-0-baseline.md](phase-0-baseline.md).

## Startup and lifecycle

The process writes a concise readiness message to stderr after its transport is
ready. Stdout remains reserved for MCP JSON-RPC in stdio mode.

Streamable HTTP exposes:

| Endpoint | Authentication | Meaning |
| --- | --- | --- |
| `GET /healthz` | none | The process can serve HTTP. |
| `GET /readyz` | none | Configuration, logging, cache, client, tools, and the listener initialized successfully. |
| `GET /metrics` | MCP bearer token | Privacy-safe Prometheus text metrics. |
| `/mcp` | MCP bearer token | Streamable HTTP MCP endpoint. |

Readiness does not call Maglev. An upstream outage should produce stable MCP
errors and may still allow cached static results; repeatedly probing Maglev
from readiness would add load and cause unnecessary deployment restarts.

On `SIGINT` or `SIGTERM`, Streamable HTTP marks readiness false, stops accepting
new connections, allows five seconds for in-flight requests to drain, cancels
remaining request contexts, and then exits. SQLite and the rotating log writer
are closed by normal process cleanup. Stdio uses the MCP SDK's signal-aware
listener and is also owned by the launching client closing stdin.

## Request correlation and logs

HTTP accepts a log-safe `X-Request-ID` containing at most 64 letters, digits,
dots, underscores, or hyphens. Missing or unsafe values are replaced with a
random ID. The ID is returned as `X-Request-ID`, included in structured MCP
success/error metadata, and carried into tool and upstream logs. Stdio tool
calls receive a generated request ID.

`caller_hash` is a short SHA-256 fingerprint of the authenticated MCP bearer
credential. It correlates use of a credential without logging the credential.
It is not an end-user identity. A production gateway remains responsible for
real user authentication, authorization, and audit identity.

Tool logs contain tool name, outcome, public error code, latency, response
size, request ID, and caller hash. Upstream logs contain operation name, cache
requested object ID when applicable, cache tier, latency, response size,
HTTP/public error classification, and request ID. They intentionally omit
URLs, coordinates, search text, API keys, bearer tokens, response bodies, and
stack traces.

## Metrics and dashboard panels

Scrape `/metrics` with the same private bearer token used between the gateway
and `oba-mcp`. Do not expose this endpoint publicly. Metrics use bounded labels:
tool, outcome, public error code, upstream operation, cache state, and HTTP
status classification. They do not contain request IDs or user data.

Recommended dashboard panels:

```promql
# Tool request rate by outcome
sum by (tool, outcome) (rate(oba_mcp_tool_requests_total[5m]))

# Tool p50 / p95 / p99 latency
histogram_quantile(0.50, sum by (le, tool) (rate(oba_mcp_tool_duration_seconds_bucket[5m])))
histogram_quantile(0.95, sum by (le, tool) (rate(oba_mcp_tool_duration_seconds_bucket[5m])))
histogram_quantile(0.99, sum by (le, tool) (rate(oba_mcp_tool_duration_seconds_bucket[5m])))

# Upstream errors and status classes
sum by (operation, status, error_code) (rate(oba_mcp_upstream_requests_total[5m]))

# Cache hit ratio
sum(rate(oba_mcp_upstream_requests_total{cache=~"hit|l2-hit"}[5m]))
/
sum(rate(oba_mcp_upstream_requests_total[5m]))

# Circuit state and active tool calls
oba_mcp_circuit_open
oba_mcp_tool_in_flight

# Calls cancelled while waiting for the bounded upstream concurrency slot
rate(oba_mcp_upstream_concurrency_limit_total{outcome="wait_cancelled"}[5m])
```

The fixed histograms also expose structured response size and upstream latency.
Use the SLO targets in `phase-0-baseline.md` when selecting dashboard thresholds.

## Alerts and runbooks

### Elevated tool error rate

Alert when upstream/internal tool failures exceed the applicable SLO for 10
minutes. Group by `error_code`. Check circuit state and upstream status first,
then correlate a sample request ID across `tool` and `upstream` logs. Invalid
arguments and caller cancellations do not indicate a service incident.

### Maglev unavailable or slow

Alert on sustained `UPSTREAM_TIMEOUT`, `UPSTREAM_UNAVAILABLE`, HTTP 5xx, or an
open circuit. Verify Maglev health and network reachability from the private
MCP network. Do not bypass the circuit breaker or increase retry counts during
an incident; doing so can amplify the outage. Static cache hits may continue.

### Cache degradation

Alert on an unexpected static cache-hit drop combined with increased upstream
traffic. Check cache-path permissions, available disk, and SQLite open errors.
An empty cache path intentionally selects bounded memory-only mode. The cache
is disposable; stop the process before replacing a corrupt database.

### Authentication failures

Investigate a sustained rise in HTTP 401/403 at the gateway or MCP boundary.
Verify secret rotation and the exact Origin allow-list. Never print tokens to
diagnose a mismatch. If compromise is suspected, rotate the MCP token and
Maglev API key independently and invalidate the old gateway secret.

### Elevated latency or saturation

Compare tool and upstream histograms, cache state, `tool_in_flight`, and
circuit state. If upstream latency is healthy, inspect the gateway and MCP
host resources. Scale within Maglev's capacity and keep the client concurrency
bound; do not remove the limit as an incident response.

## Privacy and retention

- Rotating logs are limited to 10 MiB each, retain three backups for at most
  seven days, and are gzip-compressed. Deployment log collectors must use an
  equal or shorter approved retention unless an owner documents a requirement.
- Static cache entries expire after 60 minutes. Expired SQLite rows are pruned
  on startup and before static writes; the database is capped at 32 MiB.
- Real-time responses are not cached between calls.
- Metrics contain aggregates only and must not add request ID, caller hash,
  entity ID, search query, coordinate, or response-body labels.
- Access to logs and metrics is limited to service operators. Incident exports
  must be redacted and deleted with the incident record's retention policy.

Changes to these limits require an explicit privacy and capacity review.
