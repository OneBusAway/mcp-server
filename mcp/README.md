# onebusaway-mcp-server

A Model Context Protocol server that gives LLMs live access to transit data. One Go binary, 29 tools, powered by the [OneBusAway](https://onebusaway.org) API.

Use it with Claude, Claude Code, opencode, or any MCP-compatible client to answer questions like *"When does the next bus arrive at my stop?"*, *"Where are the vehicles on route P right now?"*, or *"What's the schedule for stop 1_1013 on a weekday?"*.

## How it works

A tool call from your LLM becomes an HTTP request to a OneBusAway backend, which returns live data from GTFS static and GTFS-realtime feeds. This server handles the plumbing: parsing, caching, retries, circuit breaking, and structured responses.

## Quick start

Pick the mode that matches how you use it.

**Stdio, for Claude Code, Claude Desktop, opencode:**

```sh
make build
make mcp-add          # registers with Claude Code (user scope)
```

Start a new session in your MCP client and ask a transit question.

**HTTP, for local development or private-network gateway deployments:**

```sh
OBA_API_KEY=test OBA_HTTP_AUTH_TOKEN=local-dev-token make serve-http
```

**With explicit config files:**

```sh
make run-config CONFIG=config.json ENV_FILE=.env
```

**Docker:**

```sh
docker compose up -d
```

## Requirements

- Go 1.26.5 or newer (matches `go.mod`)
- A running [maglev](https://github.com/OneBusAway/maglev) or another OBA-compatible API reachable at `$OBA_BASE_URL`

## Configuration

Every setting can come from a JSON file, a dotenv file, or the process environment. When more than one source defines the same value, the later one wins:

```text
compiled defaults  <  config.json  <  .env  <  process environment
```

Files are optional. The server never searches the working directory; you must point at each file explicitly.

```sh
cp config.example.json config.json
cp .env.example .env

./oba-mcp --config ./config.json --env-file ./.env
# or via env vars:
OBA_CONFIG_FILE=./config.json OBA_ENV_FILE=./.env ./oba-mcp

# Validate config without starting the server:
./oba-mcp --check-config --config ./config.json --env-file ./.env

# See what the server actually loaded (secrets are always redacted):
./oba-mcp --print-config --config ./config.json --env-file ./.env
```

Use `config.json` for stable non-secret settings, `.env` for local secrets, and deployment environment variables (or a secret manager) for `OBA_API_KEY` and `OBA_HTTP_AUTH_TOKEN` in production. The real `config.json` and `.env` are gitignored; commit only the example and schema files.

For every option and the full startup story, see [CONFIGURATION.md](CONFIGURATION.md).

A note on transport: `OBA_TRANSPORT` picks how the server accepts calls (`stdio` or `streamable-http`), which is independent of the MCP protocol version. The MCP SDK negotiates compatibility on its own. The HTTP bearer token protects the service boundary; it is not a full OAuth or OIDC implementation.

### Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `OBA_CONFIG_FILE` | none | Path to `config.json` (overridden by `--config`) |
| `OBA_ENV_FILE` | none | Path to a dotenv file (overridden by `--env-file`) |
| `OBA_BASE_URL` | `http://localhost:4000` | OBA-compatible API URL |
| `OBA_API_KEY` | required | OBA API key. Inject from a secret manager in production |
| `OBA_TRANSPORT` | `stdio` | `stdio` or `streamable-http` (legacy `http` alias accepted) |
| `OBA_TOOL_PROFILE` | `all` | `all` = all 29 tools, `rider` = 16 passenger-facing tools |
| `OBA_PORT` | `8080` | HTTP listener port |
| `OBA_HTTP_BIND_ADDR` | `127.0.0.1` | HTTP listener address. Use a private network address for a gateway deployment |
| `OBA_HTTP_AUTH_TOKEN` | required in HTTP mode | Shared secret between the server and its authentication gateway |
| `OBA_ALLOWED_ORIGINS` | none | Comma-separated browser origins; all others get `403` |
| `OBA_LOG` | `/tmp/oba-mcp.log` | Log file path (rotated automatically) |
| `OBA_LOG_FORMAT` | `text` | `text` or `json` |
| `OBA_LOG_JSON` | unset | Legacy `0`/`1` alias. Conflicting log format vars are rejected |
| `OBA_CACHE` | `~/.cache/oba-mcp/cache.db` | SQLite cache path; empty means memory-only |

## Logs

Logs rotate automatically: 10 MB max per file, 3 files kept, 7-day expiry, gzip-compressed.

**Human-readable (default):**

```sh
tail -f /tmp/oba-mcp.log
# or
make logs
```

Sample output:

```
10:42:30 [START] http://localhost:4000
10:42:31 [CACHE] /home/user/.cache/oba-mcp/cache.db
10:42:32 [READY] transport=streamable-http  endpoint=http://127.0.0.1:8080/mcp
10:42:35 [MISS] arrivals-and-departures-for-stop (get_arrivals_for_stop)  ms=41  4.8KB  request_id=...
10:42:36 [L2]   route (get_route)  request_id=...
10:42:40 [OPEN]  circuit breaker  failures=3
10:42:55 [CLOSE] circuit breaker  after 3 failures
```

**JSON (for log aggregators):**

```sh
OBA_LOG_FORMAT=json ./oba-mcp
```

HTTP deployments also expose:

- `/healthz` and `/readyz`: unauthenticated liveness and readiness probes.
- `/metrics`: Prometheus metrics behind the MCP bearer token, privacy-safe.

Full operational detail (lifecycle, dashboards, runbooks, retention) is in the [Phase 6 operations guide](../docs/production/phase-6-operations.md).

## Docker

The Docker image requires explicit HTTP configuration and ships no API-key default. The Compose file binds the MCP service to loopback and does not publish the maglev container.

```sh
docker compose up -d
make docker-logs      # follow container logs
docker compose down
```

If maglev runs outside Docker (say on the host at port 4000):

```yaml
# docker-compose.yml
environment:
  OBA_BASE_URL: http://host.docker.internal:4000
```

For a file-based deployment, mount the non-secret JSON read-only and inject credentials separately:

```yaml
services:
  oba-mcp:
    command: ["--config", "/etc/oba-mcp/config.json"]
    volumes:
      - ./config.json:/etc/oba-mcp/config.json:ro
    environment:
      OBA_API_KEY: ${OBA_API_KEY:?OBA_API_KEY is required}
      OBA_HTTP_AUTH_TOKEN: ${OBA_HTTP_AUTH_TOKEN:?OBA_HTTP_AUTH_TOKEN is required}
```

## Registering with MCP clients

**Claude Code, Claude Desktop, opencode (stdio):**

```sh
make mcp-add
```

Or configure manually:

```json
{
  "command": "/path/to/oba-mcp",
  "env": {
    "OBA_BASE_URL": "http://localhost:4000",
    "OBA_API_KEY": "test"
  }
}
```

**HTTP mode** (for the transit UI or any HTTP MCP client):

```json
{
  "url": "http://localhost:8080/mcp",
  "headers": { "Authorization": "Bearer local-dev-token" }
}
```

## Public HTTP deployment

Do not expose the MCP container or a maglev backend directly to the internet.

Put a TLS + authentication gateway in front. The gateway must:

- Authenticate and authorize every caller.
- Apply per-caller and per-IP rate limits.
- Generate a request ID.
- Proxy only to the private MCP listener.
- Strip any client-supplied `Authorization` header, then add `Authorization: Bearer $OBA_HTTP_AUTH_TOKEN` on the private hop.
- Rewrite `Host` to `localhost` when proxying to a loopback listener, so the MCP library's DNS-rebinding protection stays enabled.

Keep `OBA_HTTP_AUTH_TOKEN` and `OBA_API_KEY` in the deployment secret manager. Never put them in Compose files, source control, browser storage, or tool output.

## Tools

The default `all` profile exposes the full 29-tool catalog below. Set `OBA_TOOL_PROFILE=rider` to expose only the 16 passenger-facing tools.

### Agencies

| Tool | Description |
| --- | --- |
| `get_agencies` | List all agencies |
| `get_agency` | Agency details by ID |

### Stops

| Tool | Description |
| --- | --- |
| `get_stop` | Stop by ID: name, location, routes |
| `search_stops` | Search by name or code (max 5) |
| `find_stops_near_location` | Stops within a radius of lat/lon (max 20) |
| `get_stops_for_agency` | All stops for an agency (max 50) |
| `get_stop_ids_for_agency` | All stop IDs for an agency (max 100) |
| `get_stop_schedule` | Full-day timetable: all trips and departure times |
| `get_stop_overview` | Composite: stop info + next 5 arrivals + routes in one call |

### Arrivals & Departures

| Tool | Description |
| --- | --- |
| `get_arrivals_for_stop` | Next arrivals at a stop (default: 60-min window, max 10) |
| `get_arrival_and_departure_for_stop` | Single-arrival lookup used for each per-trip tracking refresh |
| `get_arrivals_for_location` | Arrivals near a lat/lon coordinate (max 10) |

### Routes

| Tool | Description |
| --- | --- |
| `get_route` | Route by ID |
| `search_routes` | Search by name or number (max 5) |
| `get_routes_for_agency` | All routes for an agency (max 30) |
| `get_route_ids_for_agency` | All route IDs for an agency |
| `get_routes_for_location` | Routes near a lat/lon coordinate (max 20) |
| `get_stops_for_route` | Stops served by a route (max 30 per direction) |
| `get_schedule_for_route` | Route structure: directions, trip IDs, stop order |

### Trips & Vehicles

| Tool | Description |
| --- | --- |
| `get_trip` | Static trip info |
| `get_trip_details` | Real-time position, phase, schedule deviation |
| `get_trip_for_vehicle` | Active trip for a vehicle |
| `get_trips_for_route` | Active trips on a route now (max 20) |
| `get_trips_for_location` | Active trips near a lat/lon (max 20) |
| `get_vehicles_for_agency` | All active vehicles (max 50) |
| `get_block` | Full day of trips for a vehicle by block ID |

### Shapes & System

| Tool | Description |
| --- | --- |
| `get_shape` | Polyline lat/lon points for a route/trip |
| `get_current_time` | Current server time |
| `get_metadata` | Server version and GTFS feed freshness |

## Prompts

| Prompt | Description |
| --- | --- |
| `transit_assistant` | Full system prompt. Load this first for best results |
| `next_bus` | Starter for "when is the next bus?" queries |
| `explore_agency` | Starter for exploring all routes and stops |

## Caching

| Data | TTL | Persisted to SQLite? |
| --- | --- | --- |
| Static (agencies, routes, stops, shapes, schedules) | 60 min | Yes, survives across sessions |
| Real-time (arrivals, vehicle positions, trip status) | Disabled | No |

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
