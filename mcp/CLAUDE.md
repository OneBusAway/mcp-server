# CLAUDE.md — OneBusAway MCP

Before making changes, read and follow [AGENTS.md](AGENTS.md). It is the
canonical engineering guide for this MCP; this file is Claude Code context,
not a duplicate rulebook.

`oba-mcp` is a Go MCP server that exposes real-time and scheduled OneBusAway
transit data to LLMs. It runs as either a stdio subprocess or an HTTP service.

```text
LLM ←MCP→ oba-mcp ←HTTP/cache→ Maglev or another OBA-compatible API ←GTFS/GTFS-RT
                     ↑
               ../ui (optional SvelteKit UI)
```

The MCP is deployable without the UI. This directory is `mcp/`; the companion
UI is `../ui/`.

## Layout

```text
main.go                       entry point, configuration, logging, transport wiring
client/
  http_client.go              HTTP client, cache, circuit breaker, time helpers
  api_dtos.go                 typed OBA API DTOs and endpoint methods
tools/
  register.go                 shared handler and tool registration
  input.go                    shared MCP argument parsing and validation boundary
  results.go                  shared MCP result adapter (toResult)
  responses_agencies.go       agency response contracts
  responses_arrivals.go       arrival and departure response contracts
  responses_routes.go         route, shape, and schedule response contracts
  responses_stops.go          stop and stop-schedule response contracts
  responses_system.go         system and metadata response contracts
  responses_trips.go          trip, vehicle, and block response contracts
  agencies.go …               domain tool handlers
  prompts.go                  MCP prompts and transit-domain guidance
validation/                   shared argument validation
cachedb/                      sqlc-generated SQLite cache layer
```

Do not manually edit generated files in `cachedb/`.

## Development

Run commands from `mcp/`:

```sh
make build        # compile ./oba-mcp
make run          # build and run over stdio
make serve-http   # run HTTP mode on the configured address
make watch        # live reload with Air
make mcp-add      # rebuild and register with Claude Code
make logs         # tail /tmp/oba-mcp.log
make fmt && make lint && make test
```

After `make mcp-add`, start a new Claude Code conversation to use the rebuilt
server. In restricted environments, use a temporary Go cache, for example:

```sh
GOCACHE=/tmp/oba-mcp-go-cache go test ./...
GOCACHE=/tmp/oba-mcp-go-cache go vet ./...
GOCACHE=/tmp/oba-mcp-go-cache go build ./...
```

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `OBA_BASE_URL` | `http://localhost:4000` | OBA-compatible API URL |
| `OBA_API_KEY` | required | API key; inject from a secret manager |
| `OBA_TRANSPORT` | `stdio` | `stdio` or `http` |
| `OBA_TOOL_PROFILE` | `all` | `all` exposes all 29 tools; `rider` exposes the 14 passenger workflows |
| `OBA_PORT` | `8080` | HTTP listener port |
| `OBA_HTTP_BIND_ADDR` | `127.0.0.1` | HTTP listener address |
| `OBA_HTTP_AUTH_TOKEN` | required in HTTP mode | private gateway-to-MCP bearer token |
| `OBA_ALLOWED_ORIGINS` | none | exact allowed browser origins |
| `OBA_LOG` | `/tmp/oba-mcp.log` | rotated log destination |
| `OBA_LOG_JSON` | `0` | set to `1` for JSON logs |
| `OBA_CACHE` | platform cache directory | SQLite static-data cache |

HTTP mode is for a private service behind an authenticated TLS gateway, not a
public listener. Keep the API key and HTTP auth token out of source, logs, MCP
results, and browser storage.

## Adding or changing a tool

1. Inspect a production-ready tool with similar behavior and its tests.
2. Define or reuse a typed upstream OBA DTO.
3. Define or reuse a named MCP-facing response type.
4. Define the tool schema and a description that states what it returns and when to use it.
5. Validate arguments through the shared validation layer.
6. Call the context-aware client or service method.
7. Keep URL construction and upstream HTTP behavior in the client layer.
8. Transform the typed OBA DTO into the MCP-facing type.
9. Return structured MCP content through the shared result and error contracts; text is supplementary.
10. Register the tool in the appropriate registration function.
11. Update real-time/cache classification when applicable.
12. Add or update decoding, contract, and behavior tests.

The intended data boundary is:

```text
OBA response → typed OBA DTO → explicit mapping → named MCP response → structured MCP result
```

Do not turn untrusted arguments into URL paths in handlers. Do not use dynamic
maps or unchecked assertions for supported OBA responses. Do not create
anonymous handler-local structs as public response contracts.

## OneBusAway response model

OBA commonly returns an envelope such as:

```json
{
  "code": 200,
  "data": {
    "entry": {},
    "list": [],
    "references": {}
  }
}
```

Supported endpoints must decode this into named DTOs in `client/`, rather than
parsing it dynamically. The primary payload often contains IDs while shared
stops, routes, trips, and agencies appear under `references`; model those
references in typed DTOs and explicitly transform only the needed values into
the MCP-facing response.

## Transit-domain notes

### Arrivals

Useful upstream arrival fields include:

```text
predicted               true when real-time GPS data is used
predictedArrivalTime    Unix milliseconds; zero when no prediction exists
scheduledArrivalTime    Unix milliseconds
scheduleDeviation       seconds late (+) or early (-)
numberOfStopsAway       remaining stops when known
distanceFromStop        metres
tripId, routeId         transit identities
serviceDate             midnight Unix milliseconds for the operating service day
```

`serviceDate` is the transit operating day, which can differ from the calendar
date around midnight. Respect the agency timezone for display and for strict
schedule-date queries.

### Timestamps and timezones

OBA timestamps are Unix **milliseconds**, not seconds. Use `time.UnixMilli`
when producing a display value; `FormatRelativeTime` can produce text such as
`3:42 PM (in 8 min)`.

Those strings are display helpers only. MCP-facing responses should preserve
machine-readable epoch milliseconds and applicable timezone information so a
client or model never has to parse display text to recover a time.

The OBA `time` parameter accepts epoch milliseconds for past or future
queries. Agency data supplies the timezone; obtain it through the client when
the endpoint does not already provide an applicable offset.

### Limits, caching, and logs

Bound list output. Current tool limits include searches of 5; arrivals 10;
routes 30; stops 50; nearby stops/routes 20; vehicles 50; trips 20; and stop
IDs 100. Indicate truncation when a result is capped.

Static data such as agencies, routes, stops, shapes, and schedules is cached
for 60 minutes and may persist in SQLite. Real-time arrivals, vehicle/trip
status, and current time are cached for 30 seconds in memory only.

Logs rotate at 10 MB, retain three compressed files for seven days, and are
human-readable by default. Set `OBA_LOG_JSON=1` for log aggregation.

## Tool design and errors

Tool descriptions are model-facing API documentation. State what a tool
returns, when it is appropriate, and a preferable alternative when one exists.
Repeated A → B → C workflows may justify a focused composite tool, such as a
stop overview, rather than forcing the model to chain low-level calls.

Structured MCP content is the canonical machine-readable result. A concise
human-readable summary may supplement it, but clients must not scrape JSON
from text.

Expected tool failures should be returned as MCP tool results when an SDK-level
error could close or destabilize the MCP connection. Translate internal and
upstream failures into the stable public MCP error contract; never expose raw
`err.Error()`, stack traces, internal URLs, or secrets to MCP clients.

## Claude Code workflow

### Before implementing

1. Read `AGENTS.md`.
2. Inspect the relevant implementation and tests.
3. Understand the current architecture and public contract.
4. Prefer established production-ready patterns over new abstractions.
5. Keep the change scoped to the request.

### Before finishing

1. Run the checks required by `AGENTS.md`.
2. Review the complete diff.
3. Remove unnecessary abstractions, comments, debug code, and unrelated changes.
4. Verify public MCP contracts were not unintentionally changed.
5. Report checks that could not be run.
