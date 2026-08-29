# oba-mcp

An MCP (Model Context Protocol) server that wraps the [OneBusAway](https://onebusaway.org) REST API, giving LLMs real-time access to transit data — arrivals, routes, stops, vehicles, and schedules.

```
LLM (Claude, etc.)  ←MCP→  oba-mcp  ←HTTP→  maglev (OBA API)  ←GTFS/GTFS-RT
```

## Quick Start

```sh
# Local (stdio mode — used by Claude Code, Claude Desktop, opencode)
make build
make mcp-add          # registers with Claude Code, all projects

# Local HTTP mode — requires a bearer token, for development only
OBA_API_KEY=test OBA_HTTP_AUTH_TOKEN=local-dev-token make serve-http

# Docker
docker compose up -d
```

## Requirements

- Go 1.23+ (local builds)
- A running [maglev](https://github.com/OneBusAway/maglev) or OBA-compatible API at `$OBA_BASE_URL`

## Configuration

| Env var | Default | Description |
|---|---|---|
| `OBA_BASE_URL` | `http://localhost:4000` | OBA-compatible API URL |
| `OBA_API_KEY` | required | OBA API key; inject it from a deployment secret |
| `OBA_TRANSPORT` | `stdio` | `stdio` (MCP clients) or `http` (web/HTTP clients) |
| `OBA_PORT` | `8080` | Port for HTTP mode |
| `OBA_HTTP_BIND_ADDR` | `127.0.0.1` | HTTP listener address; use a private network address for a gateway deployment |
| `OBA_HTTP_AUTH_TOKEN` | required in HTTP mode | Secret shared only by the MCP server and its authentication gateway |
| `OBA_ALLOWED_ORIGINS` | none | Comma-separated exact browser origins; all other browser origins receive `403` |
| `OBA_LOG` | `/tmp/oba-mcp.log` | Log file path (rotated automatically) |
| `OBA_LOG_JSON` | `0` | Set to `1` for raw JSON logs (default: human-readable) |
| `OBA_CACHE` | `~/.cache/oba-mcp/cache.db` | SQLite persistent cache |

## Logs

Logs rotate automatically (10 MB max, 3 files kept, 7-day expiry, gzip-compressed).

**Human-readable (default):**
```
tail -f /tmp/oba-mcp.log
# or
make logs
```

Sample output:
```
10:42:30 [START] http://localhost:4000
10:42:31 [CACHE] /home/ahmed/.cache/oba-mcp/cache.db
10:42:35 [MISS]  arrivals-and-departures-for-stop          ms=41  4.8KB
10:42:35 [HIT]   stop
10:42:36 [L2]    route
10:42:40 [OPEN]  circuit breaker  failures=3
10:42:55 [CLOSE] circuit breaker  after 3 failures
```

**JSON (for log aggregators):**
```sh
OBA_LOG_JSON=1 ./oba-mcp
```

## Docker

The Docker image requires explicit HTTP configuration and contains no API-key default. The Compose file binds MCP to loopback for local development and does not publish Maglev.

```sh
# Start
docker compose up -d

# Follow logs
make docker-logs

# Stop
docker compose down
```

If maglev runs outside Docker (e.g. on the host at port 4000):
```yaml
# in docker-compose.yml
environment:
  OBA_BASE_URL: http://host.docker.internal:4000
```

## Register with MCP Clients

**Claude Code / Claude Desktop / opencode (stdio):**
```sh
make mcp-add
```

Or configure manually:
```json
{
  "command": "/path/to/oba-mcp",
  "env": {
    "OBA_BASE_URL": "http://localhost:4000",
    "OBA_API_KEY":  "test"
  }
}
```

**HTTP mode** (transit-ui or other HTTP MCP clients):
```json
{
  "url": "http://localhost:8080/mcp",
  "headers": { "Authorization": "Bearer local-dev-token" }
}
```

## Public HTTP deployment

Do not expose the MCP container or Maglev directly to the internet. Put a TLS
and authentication gateway in front of the MCP server. The gateway must
authenticate and authorize each caller, apply caller/IP rate limits, generate a
request ID, and proxy only to the private MCP listener. It must strip any
client-supplied `Authorization` header and add `Authorization: Bearer
$OBA_HTTP_AUTH_TOKEN` only on the private upstream connection. When proxying to
a loopback listener, rewrite `Host` to `localhost` so the MCP library's DNS
rebinding protection remains enabled. Keep `OBA_HTTP_AUTH_TOKEN` and
`OBA_API_KEY` in the deployment secret manager; never put either in Compose,
source control, browser storage, or tool output.

## Tools (29 total)

### Agencies
| Tool | Description |
|------|-------------|
| `get_agencies` | List all agencies |
| `get_agency` | Agency details by ID |

### Stops
| Tool | Description |
|------|-------------|
| `get_stop` | Stop by ID — name, location, routes |
| `search_stops` | Search by name or code (max 5) |
| `find_stops_near_location` | Stops within a radius of lat/lon (max 20) |
| `get_stops_for_agency` | All stops for an agency (max 50) |
| `get_stop_ids_for_agency` | All stop IDs for an agency (max 100) |
| `get_stop_schedule` | Full-day timetable — all trips and departure times |
| `get_stop_overview` | Composite: stop info + next 5 arrivals + routes in one call |

### Arrivals & Departures
| Tool | Description |
|------|-------------|
| `get_arrivals_for_stop` | Next arrivals at a stop (default: 30 min window, max 10) |
| `get_arrival_and_departure_for_stop` | Single arrival for a specific trip |
| `get_arrivals_for_location` | Arrivals near a lat/lon coordinate (max 10) |

### Routes
| Tool | Description |
|------|-------------|
| `get_route` | Route by ID |
| `search_routes` | Search by name or number (max 5) |
| `get_routes_for_agency` | All routes for an agency (max 30) |
| `get_route_ids_for_agency` | All route IDs for an agency |
| `get_routes_for_location` | Routes near a lat/lon coordinate (max 20) |
| `get_stops_for_route` | Stops served by a route (max 30 per direction) |
| `get_schedule_for_route` | Route structure: directions, trip IDs, stop order |

### Trips & Vehicles
| Tool | Description |
|------|-------------|
| `get_trip` | Static trip info |
| `get_trip_details` | Real-time: position, phase, schedule deviation |
| `get_trip_for_vehicle` | Active trip for a vehicle |
| `get_trips_for_route` | Active trips on a route now (max 20) |
| `get_trips_for_location` | Active trips near a lat/lon (max 20) |
| `get_vehicles_for_agency` | All active vehicles (max 50) |
| `get_block` | Full day of trips for a vehicle by block ID |

### Shapes & System
| Tool | Description |
|------|-------------|
| `get_shape` | Polyline lat/lon points for a route/trip |
| `get_current_time` | Current server time |
| `get_metadata` | Server version and GTFS feed freshness |

## Prompts (3 total)

| Prompt | Description |
|--------|-------------|
| `transit_assistant` | Full system prompt — load this first for best results |
| `next_bus` | Starter for "when is the next bus?" queries |
| `explore_agency` | Starter for exploring all routes and stops |

## Caching

| Data | TTL | Written to SQLite? |
|---|---|---|
| Static (agencies, routes, stops, shapes, schedules) | 60 min | Yes — persists across sessions |
| Real-time (arrivals, vehicle positions, trip status) | 30 sec | No |

## Makefile

```sh
make build          # compile binary
make run            # build + run (stdio)
make serve-http     # run in HTTP mode
make watch          # live-reload via Air
make mcp-add        # register with Claude Code
make mcp-remove     # unregister from Claude Code
make logs           # tail /tmp/oba-mcp.log
make test           # go test ./...
make fmt            # go fmt ./...
make lint           # go vet ./...
make docker-build   # build Docker image
make docker-up      # start via Docker Compose
make docker-down    # stop containers
make docker-logs    # follow container logs
```
