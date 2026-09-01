# OneBusAway MCP — Monorepo

Real-time transit data for LLMs via the [OneBusAway](https://onebusaway.org) API.

```
LLM (Claude, etc.)  ←MCP→  onebusaway-mcp  ←HTTP→  maglev (OBA API)  ←GTFS/GTFS-RT
                                  ↑
                      onebusaway-mcp-ui  (optional web chat UI)
```

## Packages

| Directory | Description |
|-----------|-------------|
| [`mcp/`](./mcp/) | Go MCP server — exposes 29 transit tools over stdio or HTTP |
| [`ui/`](./ui/) | SvelteKit web UI — chat interface that calls the MCP server |

## Quick Start

### MCP server only (for Claude Code / Claude Desktop / opencode)

```sh
cd mcp
make build
make mcp-add        # registers with Claude Code at user scope
```

### MCP server + Web UI (local dev)

```sh
# Terminal 1 — start MCP server in HTTP mode
cd mcp
make serve-http     # listens on :8080

# Terminal 2 — start the web UI
cd ui
npm install
npm run dev         # opens at http://localhost:5173
```

### Docker (both services)

```sh
docker compose up -d
```

This starts:
- `oba-mcp` on port 8080 (MCP HTTP server)
- `maglev` on the internal Compose network (OBA API backend — configure or replace as needed)

The optional web UI is kept out of the default deployment. Start it explicitly
with `docker compose --profile ui up -d` after configuring a secure HTTP gateway.

## Requirements

- **MCP server:** Go 1.26.5+
- **Web UI:** Node.js 20+
- **Backend:** A running [maglev](https://github.com/OneBusAway/maglev) or OBA-compatible API

## Configuration

Set `OBA_BASE_URL` and `OBA_API_KEY` to point at your OBA backend. See each package's README for the full list of options.

## Production baseline

The default is trusted local stdio. The current HTTP mode is for local development
only and must not be exposed publicly until the production hardening phases are
complete. Deployment modes, tool profiles, compatibility rules, and initial
service-level objectives are documented in [the Phase 0 production
baseline](./docs/production/phase-0-baseline.md).

## Docker Compose

The root `docker-compose.yml` wires all three services together. To deploy only the MCP server:

```sh
docker compose up -d oba-mcp
```

To point at a maglev instance running outside Docker:

```yaml
# docker-compose.yml
environment:
  OBA_BASE_URL: http://host.docker.internal:4000
```
