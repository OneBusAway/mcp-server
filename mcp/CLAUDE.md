# CLAUDE.md — onebusaway-mcp

MCP server wrapping the OneBusAway REST API. Exposes real-time transit data as tools for LLMs. Runs as a stdio subprocess or HTTP server.

```
LLM  ←MCP→  oba-mcp  ←HTTP+cache→  maglev (OBA API)  ←GTFS/GTFS-RT
                ↑
   ../ui  (optional SvelteKit web UI)
```

This package lives at `mcp/` inside the monorepo. The companion web UI is at `../ui/` — see its [README](../ui/README.md) and [CLAUDE.md](../ui/CLAUDE.md) if one exists.

## Project Layout

```
mcp/                         # ← you are here
  main.go              # entry point: env, logging, cache, server wiring
  client/oba.go        # HTTP client, 2-level cache (memory + SQLite), circuit breaker
  logger/pretty.go     # human-readable log formatter (pretty default, JSON with OBA_LOG_JSON=1)
  cachedb/             # sqlc-generated SQLite layer — never edit db.go, models.go, query.sql.go
  tools/
    register.go        # Handler struct + RegisterAll()
    agencies.go        # get_agencies, get_agency
    arrivals.go        # get_arrivals_for_stop, get_arrival_and_departure_for_stop, get_arrivals_for_location
    overview.go        # get_stop_overview (composite: stop + arrivals in one call)
    routes.go          # get_route, search_routes, get_routes_for_agency, …
    stops.go           # get_stop, search_stops, find_stops_near_location, …
    system.go          # get_current_time, get_metadata
    trips.go           # get_trip, get_trip_details, get_trip_for_vehicle, …
    prompts.go         # MCP prompts: transit_assistant, next_bus, explore_agency

../ui/        # companion SvelteKit web chat UI
  src/
    lib/
      mcp.js           # MCP HTTP client (callTool, listTools)
      chat.svelte.js   # global chat store
      settings.svelte.js
      components/      # ArrivalsPanel, MapCard, ArrivalRow, …
    routes/
      chat/+page.svelte
      api/chat/+server.js  # SSE bridge: UI ↔ MCP ↔ AI provider
```

## Development Commands

```sh
make build        # compile ./oba-mcp
make run          # build + run (stdio)
make serve-http   # run in HTTP mode (:8080)
make watch        # Air live-reload
make mcp-add      # rebuild + register with Claude Code (user scope)
make logs         # tail -f /tmp/oba-mcp.log
make fmt && make lint && make test
```

After `make mcp-add`, start a new conversation to pick up the new binary.

## Environment Variables

| Var | Default | Notes |
|---|---|---|
| `OBA_BASE_URL` | `http://localhost:4000` | maglev URL |
| `OBA_API_KEY` | `test` | OBA API key |
| `OBA_TRANSPORT` | `stdio` | `stdio` or `http` |
| `OBA_PORT` | `8080` | port for HTTP mode |
| `OBA_LOG` | `/tmp/oba-mcp.log` | log file (auto-rotated: 10MB, 3 files, 7 days) |
| `OBA_LOG_JSON` | `0` | set `1` for raw JSON logs |
| `OBA_CACHE` | `~/.cache/oba-mcp/cache.db` | SQLite cache |

## Adding a New Tool

**1. Register in the file's `registerXxxTools` function:**

```go
s.AddTool(
    mcp.NewTool("get_thing",
        mcp.WithDescription("One sentence: what it returns and when to prefer it over alternatives."),
        mcp.WithString("thing_id", mcp.Required(), mcp.Description("Thing ID (e.g. 'unitrans_abc')")),
        mcp.WithNumber("time", mcp.Description("Query at specific time (epoch ms, defaults to now)")),
    ),
    h.getThing,
)
```

**2. Write the handler (follow this shape exactly):**

```go
func (h *Handler) getThing(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    thingID, err := req.RequireString("thing_id")
    if err != nil || thingID == "" {
        return mcp.NewToolResultError("thing_id is required"), nil
    }

    params := url.Values{}
    if t := req.GetFloat("time", 0); t > 0 {
        params.Set("time", fmt.Sprintf("%.0f", t))
    }

    resp, err := h.client.Get("/api/where/thing/"+thingID+".json", params)
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }
    data, err := client.Data(resp)
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }

    entry, _ := data["entry"].(map[string]any)
    if entry == nil {
        return mcp.NewToolResultText(fmt.Sprintf("No thing found with ID %q.", thingID)), nil
    }

    type result struct {
        ID   string `json:"id"`
        Name string `json:"name,omitempty"`
    }
    out, _ := json.MarshalIndent(result{
        ID:   client.StrVal(entry["id"]),
        Name: client.StrVal(entry["name"]),
    }, "", "  ")
    return mcp.NewToolResultText(fmt.Sprintf("Thing %s:\n%s", thingID, out)), nil
}
```

**3.** Call `h.registerThingTools(s)` inside `RegisterAll` in `register.go`.

**4.** If real-time data, add the path prefix to `realtimePrefixes` in `client/oba.go`.

## OBA API

### Response Envelope

```json
{ "code": 200, "data": { "entry": {…}, "list": […], "references": { "stops": […], "routes": […] } } }
```

`client.Data(resp)` validates `code==200` and returns the inner `data` map.

Single resource → `data["entry"].(map[string]any)` | List → `client.AsSlice(data["list"])`

### References Lookup

Many endpoints put shared objects in `references` and only IDs in the payload:

```go
stopNames := map[string]string{}
if refs, ok := data["references"].(map[string]any); ok {
    for _, s := range client.AsSlice(refs["stops"]) {
        stop, _ := s.(map[string]any)
        stopNames[client.StrVal(stop["id"])] = client.StrVal(stop["name"])
    }
}
```

### Arrivals — Key Fields

```
predicted               bool    — true = real-time GPS data
predictedArrivalTime    float64 — epoch ms (0 if unpredicted)
scheduledArrivalTime    float64 — epoch ms (always present)
scheduleDeviation       float64 — seconds late (+) or early (-)
numberOfStopsAway       float64
distanceFromStop        float64 — meters
tripId / serviceDate    string / float64 — serviceDate = midnight epoch ms of the operating day
```

### Timestamps

OBA timestamps are Unix **milliseconds** (float64). Always format before returning:

```go
client.FormatRelativeTime(ms, loc)              // "3:42 PM (in 8 min)"
time.UnixMilli(int64(ms)).Format("3:04 PM")    // bare time
time.UnixMilli(int64(ms)).Format("2006-01-02") // date

// Timezone for an agency (cached after first call):
loc := h.client.TimezoneFor(client.AgencyIDFromEntityID(stopID))
```

The `time=` parameter (epoch ms) lets the LLM query at any past/future time. The system prompt in `prompts.go` explains the computation.

## Go Style Rules

- **Typed structs always** — never return `map[string]any` to the LLM; define a struct and marshal it.
- **`omitempty` on zero-value fields** — initialize slices as `nil` (not `[]string{}`), or `omitempty` won't fire.
- **Use client helpers** — `client.StrVal`, `client.FloatVal`, `client.AsSlice` — never type-assert directly (panics on nil).
- **Cap list results** — arrivals=10, routes=30, stops=50, stop IDs=100, search=5, vehicles=50, trips=20, stops-near=20.
- **Errors to LLM** — `return mcp.NewToolResultError(err.Error()), nil`. Never `return nil, err` (closes the MCP connection).
- **No comments on obvious things** — only comment when the WHY is non-obvious.

## Cache

| TTL | Data | SQLite? |
|---|---|---|
| 30 sec | arrivals, trip-details, vehicles, current-time | No |
| 60 min | agencies, routes, stops, shapes, schedules | Yes — cross-session |

## Logging

Auto-rotates: 10 MB → rotate, 3 files kept, 7-day expiry, gzip.
Default: human-readable. `OBA_LOG_JSON=1` → raw JSON.

```
10:42:35 [MISS]  arrivals-and-departures-for-stop  ms=41  4.8KB
10:42:35 [HIT]   stop
10:42:40 [OPEN]  circuit breaker  failures=3
```

## Tool Design

- **Description = API docs** — model reads only name + description before choosing a tool. Say what it returns AND when to prefer it over alternatives.
- **Mention alternatives explicitly** — "Use `get_stops_for_agency` if you also need names and locations."
- **Response format** — human-readable header then JSON: `fmt.Sprintf("Arrivals at stop %s (%d shown):\n%s", stopID, len(results), out)`.
- **Composite tools** — if model always calls A→B→C for the same question, build a composite (see `get_stop_overview`).
