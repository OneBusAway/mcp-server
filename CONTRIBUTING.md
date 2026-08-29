# Contributing to OneBusAway MCP

Thanks for contributing. This repository contains an MCP server in `mcp/` and
an optional web UI in `ui/`. The MCP server must remain deployable without the
UI and must work with any compatible OneBusAway REST API implementation.

## Before you start

- Keep a change focused on one user or operational need.
- Do not commit API keys, bearer tokens, local cache files, or generated
  binaries.
- Preserve the default `stdio` transport. HTTP deployments must remain behind
  an authenticated TLS gateway.
- Update the relevant production TODO or documentation when a change completes
  a planned hardening item.

## Typed API boundary

Tool handlers must not decode arbitrary JSON or pass raw upstream payloads to
an MCP client. The server uses two explicit type layers:

```text
OneBusAway REST API
        ↓
client API DTOs (`mcp/client/api_dtos.go`)
        ↓ explicit handler mapping
MCP response types (`mcp/tools/responses.go`)
        ↓
LLM / MCP client
```

### API DTOs

Types in `mcp/client/api_dtos.go` model the portion of the OneBusAway REST API
that the MCP needs. They use the API's field names and JSON tags, such as
`routeId` and `tripHeadsign`. These types are intentionally implementation
neutral: the backend may be Maglev, onebusaway-application-modules, or another
compatible OBA server.

Add or change an API DTO when the upstream REST contract changes or when a
tool needs another upstream field. Keep the type in the `client` package.

### MCP response types

Types in `mcp/tools/responses.go` are the public contract returned to an LLM.
They use concise, stable fields and MCP-oriented JSON names, such as
`route_id` and `headsign`. They must contain only information useful for the
agent's task; do not expose full OBA reference collections, internal metadata,
or unused enum fields.

Add a named response type for every new tool response. Map the API DTO to this
type in the handler. This explicit mapping is where we choose what an agent is
allowed to see and keeps MCP clients insulated from upstream API changes.

### Rules

- Do not use `map[string]any`, `any` assertions, or presentation-text parsing
  for OBA API data in a tool handler.
- Do not return an upstream OBA response directly to the MCP client.
- Do not define anonymous handler-local response structs. Put named public
  response types in `mcp/tools/responses.go`.
- Keep responses bounded. Use a documented limit, filtering, or pagination for
  potentially large results.
- Prefer one task-focused MCP tool over exposing a REST endpoint verbatim.
- Treat every tool argument as untrusted input and validate IDs, coordinates,
  dates, ranges, and limits before making upstream calls.

## Adding or changing a tool

1. Decide the agent task and ensure it does not overlap with an existing tool.
2. Add the tool registration, input schema, clear usage description, and input
   validation in `mcp/tools/`.
3. Add the necessary upstream DTOs and typed client method in `mcp/client/`.
4. Add or update the named MCP response type in `mcp/tools/responses.go`.
5. Map API DTOs to the MCP response explicitly in the handler.
6. Add tests for upstream decoding and the public response contract.
7. Update the tool list and any user-facing documentation.

## Tests and checks

Run these from `mcp/` before submitting a change:

```sh
go test ./...
go vet ./...
go build ./...
```

When the environment has a read-only default Go build cache, use a temporary
cache instead:

```sh
GOCACHE=/tmp/oba-mcp-go-cache go test ./...
GOCACHE=/tmp/oba-mcp-go-cache go vet ./...
GOCACHE=/tmp/oba-mcp-go-cache go build ./...
```

Tests should use the named DTO and MCP response types. Cover successful,
empty, malformed, and upstream-error responses where applicable. Add a
task-level evaluation when a change affects tool selection, descriptions, or
arguments.

## Review checklist

- Is the tool’s purpose clear and distinct?
- Are all input and output contracts explicitly typed?
- Is the LLM response small, deterministic, and free of raw backend data?
- Are sensitive values excluded from output and logs?
- Do tests, vet, and build pass?
