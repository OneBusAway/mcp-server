# UI architecture

SvelteKit chat client for the [OneBusAway MCP server](../mcp/). The UI holds no transit domain knowledge of its own — every fact rendered originates from a structured MCP tool result. This document is the map contributors should read before editing.

For the dev loop and PR rules see [CONTRIBUTING.md](./CONTRIBUTING.md).

## Runtime shape

```
Browser ──HTTP──► SvelteKit server ──HTTP──► oba-mcp ──HTTP──► Maglev / OBA
                        │
                        └──HTTP──► LLM provider (Anthropic / OpenAI / Ollama / …)
```

- The browser talks only to the SvelteKit server on same-origin routes (`/api/*`).
- The SvelteKit server holds two secrets: `OBA_MCP_AUTH_TOKEN` (the MCP bearer) and the LLM provider API key selected by the user.
- The MCP bearer and the upstream OBA API key never reach the browser. The provider API key is stored in `localStorage` and sent server-side per request; it is not persisted on the server.

## Module map

```
src/
  app.html                     HTML shell
  app.css                      Tailwind base
  routes/
    +layout.svelte             App shell (sidebar + main)
    chat/+page.svelte          Chat page: message list, tool cards, map surface
    settings/+page.svelte      Provider, model, API key, theme
    api/
      mcp/+server.js           Same-origin MCP proxy (bearer-authenticated)
      chat/+server.js          SSE endpoint: LLM ↔ MCP tool loop
      chat/dispatch.js         Tool-name → card handler dispatch
      chat/map.js              Server-side geometry helpers (polyline decode)
      chat/stream.js           SSE frame helpers
  lib/
    chat.svelte.js             Global chat store (messages, streaming state, history)
    mcp.js                     Typed client wrapper over /api/mcp
    result.js                  MCP result helpers (structured/text extraction)
    settings.svelte.js         Persistent UI settings
    theme.svelte.js            Dark-mode toggle
    tracking.svelte.js         Arrival-tracking orchestrator (notifications, polling)
    components/
      ArrivalRow.svelte        Single arrival line
      ArrivalsPanel.svelte     Live arrival board for a stop
      BusLoader.svelte         Loading animation
      Header.svelte            App header
      Icon.svelte              Lucide icon wrapper
      MapCard.svelte           MapLibre map with stop/route/vehicle markers
      StatusDot.svelte         Colored status indicator
      ThemeToggle.svelte       Light/dark switch
      TrackingWidget.svelte    In-chat tracking control
      TrackOffer.svelte        "Track this bus?" prompt
```

### Target module map

Aspirational — the tree should grow deeper and thinner as MCP client, LLM providers, cards, and map engines split into their own modules:

```
src/lib/
  mcp/
    schemas.js                 Zod validators for every tool's structuredContent
    client.js                  Per-tool typed wrappers (renamed from mcp.js)
  llm/
    providers/
      anthropic.js             streamChat({messages, tools}) adapter
      openai.js
      ollama.js
    registry.js                Provider lookup
  cards/
    arrivals.js                fromToolResult(result) → Card | null
    route-map.js
    stop-overview.js
    …
  map/
    engine.d.ts                MapEngine interface (typed via JSDoc)
    registry.js                createMapEngine(providerId, opts)
    geometry.js                Polyline decode, snap, interpolate
    markers.js                 Provider-neutral DOM element factory
    providers/
      maplibre/index.js
      mapbox/index.js
      leaflet/index.js
  chat/
    coalesce.js                Card coalescing (moved from chat.svelte.js)
  tracking/
    orchestrator.svelte.js     Orchestration only
    notifications.js           Chime + Notification API
    arrival-status.js          Pure window/miss detection
```

The current tree is the starting point; the target tree above is the direction to extract toward as modules grow past ~250 lines.

## Data flow

### Chat turn

```
1. User types → chat.svelte.js appends user message
2. UI POSTs /api/chat with the message history
3. /api/chat streams from the LLM provider, exposing tool_use events
4. For each tool_use, /api/chat calls the same-origin /api/mcp proxy
5. /api/mcp adds the MCP bearer + X-Request-ID, forwards to the MCP
6. MCP returns a structured envelope (SuccessEnvelope or ErrorEnvelope)
7. dispatch.js validates the envelope and produces one or more Cards
8. /api/chat streams tool_result + card events back over SSE
9. UI stores the assistant message with attached cards; chat/+page.svelte renders
```

### MCP call

```
mcp.js (typed client)
  → /api/mcp/+server.js (adds bearer + X-Request-ID)
    → oba-mcp/mcp (validates bearer, applies middleware, executes tool)
      → structured MCP result with { data, meta, warnings? } or { code, message, retryable, retry_after_ms?, request_id? }
```

The `request_id` and `meta.request_id` phase-6 fields are propagated back to the UI for support-report display.

## MCP contract boundaries

The UI reads only from `structuredContent`. Text summaries returned alongside are for display or accessibility; they are never parsed. The envelope shapes below are authoritative:

```jsonc
// SuccessEnvelope
{
  "data":     { /* per-tool typed payload */ },
  "meta":     { "generated_at_ms": 0, "truncated": false, "cache": "hit|miss|l2-hit|", "request_id": "…" },
  "warnings": ["…"]
}

// ErrorEnvelope
{
  "code":           "UPSTREAM_TIMEOUT" /* | INVALID_ARGUMENT | UPSTREAM_UNAVAILABLE | UPSTREAM_CIRCUIT_OPEN | UPSTREAM_RATE_LIMITED | UPSTREAM_BAD_RESPONSE | UPSTREAM_RESPONSE_TOO_LARGE | UPSTREAM_CANCELLED | OUTPUT_TOO_LARGE | INTERNAL_ERROR */,
  "message":        "Human-safe explanation",
  "retryable":      true,
  "retry_after_ms": 5000,
  "request_id":     "…"
}
```

Field-by-field mapping and the phase-6 `request_id` propagation live in [phase-6-operations.md](../docs/production/phase-6-operations.md). The Go source of truth is [`../mcp/tools/results.go`](../mcp/tools/results.go).

## Coding rules

Load-bearing rules enforced in review:

1. **Structured content is the only contract.** Never string-match tool text summaries. Never regex on error messages — branch on `code`.
2. **Every MCP call goes through a Zod schema.** Unknown optional fields must be ignored, not rejected — forward compatibility with new MCP fields.
3. **Providers behind narrow interfaces.** LLM adapters expose one `streamChat`; map providers expose one `MapEngine`. Adding a provider means one new file plus a registry entry.
4. **No file above ~250 lines.** Split by responsibility, not by convenience.
5. **Pure functions for logic; effects at the edge.** Geometry, schemas, window calculation, coalescing — pure and unit-tested. `$effect`, `fetch`, and MapLibre calls live in the shell.
6. **No dead code, no narrator comments.** Delete rather than deprecate. A comment explains a non-obvious _why_.
7. **Never mutate incoming props.** Return a new object.
8. **Server-only secrets stay server-only.** `OBA_MCP_AUTH_TOKEN` and any upstream key must never appear in a client bundle, in `structuredContent`, or in `localStorage`.

## Testing surface

- **Unit** (`src/**/*.test.js`) — pure logic: `mcp/schemas.js`, `map/geometry.js`, `cards/*.fromToolResult`, `tracking/arrival-status.js`, `chat/coalesce.js`, LLM adapters replaying recorded provider streams.
- **Component** (`src/**/*.test.svelte.js`) — `ArrivalsPanel`, `MapCard` with a mocked `MapEngine`.
- **E2E** (`tests/e2e/*.spec.js`) — golden paths, provider swap, map-provider swap.
- **Contract fixtures** (`tests/fixtures/mcp/<tool>.json`) — one per MCP tool, with a "future field" case and a "legacy" case.

## Deployment shape

- The UI is a Node adapter build (`@sveltejs/adapter-node`).
- The Docker image at [`Dockerfile`](./Dockerfile) is built into the root `docker-compose.yml`.
- Required env at runtime: `OBA_MCP_URL`, `OBA_MCP_AUTH_TOKEN`.
- The MCP host must be reachable from the UI container on the same private network as the gateway; both must share the bearer.

## Extending the UI

- **New MCP tool** — add a Zod schema in `src/lib/mcp/schemas.js`, a typed wrapper in `src/lib/mcp/client.js`, a card handler in `src/lib/cards/<tool>.js`, and a fixture in `tests/fixtures/mcp/<tool>.json`. Register the card in `dispatch.js`.
- **New LLM provider** — add `src/lib/llm/providers/<name>.js` implementing `streamChat`. Register in `providers/index.js`. Add a Settings option.
- **New map provider** — add `src/lib/map/providers/<name>/index.js` implementing `MapEngine`. Register in `map/registry.js`. Add a `mapProvider` option in Settings.
- **New card type** — add `src/lib/cards/<name>.js` exporting `fromToolResult(result) → Card | null`. Register in `dispatch.js`. Add a component under `src/lib/components/` if the card renders differently from existing ones.
