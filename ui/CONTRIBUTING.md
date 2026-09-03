# Contributing to the OBA MCP UI

The UI is a SvelteKit chat interface for the [OneBusAway MCP server](../mcp/). This guide covers the dev loop, tests, and provider setup. Architecture and module layout live in [ARCHITECTURE.md](./ARCHITECTURE.md).

## Prerequisites

- Node.js 20+
- `mcp` running in HTTP mode. From `../mcp/`:
  ```sh
  OBA_API_KEY=test OBA_HTTP_AUTH_TOKEN=local-dev make serve-http
  ```
- An LLM provider — an Anthropic or OpenAI API key entered in the UI Settings page, or a local Ollama / llama-server instance.

## Dev loop

```sh
npm install
MCP_URL=http://localhost:8080 MCP_AUTH_TOKEN=local-dev make dev
```

The UI listens on http://localhost:5173. Provider settings and API key persist in `localStorage`; the MCP bearer token and OBA API key stay server-side and never reach the browser.

## npm scripts

| Script             | Purpose                                                  |
| ------------------ | -------------------------------------------------------- |
| `npm run dev`      | Vite dev server with HMR                                 |
| `npm run build`    | Production build                                         |
| `npm run preview`  | Preview the production build                             |
| `npm run check`    | `svelte-check` + `tsc --noEmit` over JSDoc-typed sources |
| `npm run lint`     | ESLint (flat config)                                     |
| `npm run format`   | Prettier write                                           |
| `npm run test`     | Vitest unit and component tests                          |
| `npm run test:e2e` | Playwright end-to-end tests                              |

`check`, `lint`, `test`, and `build` all run in CI on every pull request. `test:e2e` runs on the same job once Playwright fixtures are in place.

## Coding conventions

The full ruleset lives in [ARCHITECTURE.md](./ARCHITECTURE.md#coding-rules). The load-bearing ones:

- **Structured MCP content is the only contract.** Never parse or string-match tool text summaries.
- **Zod-validate every MCP response** at the parse site (`src/lib/mcp/schemas.js`). Unknown optional fields must be ignored, not rejected.
- **No file above ~250 lines.** Pure functions for parsing, geometry, and state transitions.
- **No narrator comments.** A comment should explain a non-obvious _why_, never a _what_.
- **Delete rather than deprecate.** No `// removed` markers, no legacy fallback branches.

## Testing

- **Unit** (`src/**/*.test.js`) — Vitest with jsdom. Pure functions: schemas, geometry, arrival-window logic, LLM stream adapters.
- **Component** (`src/**/*.test.svelte.js`) — Vitest + `@testing-library/svelte`. Render, mocked MCP fixture, assert what the user sees.
- **E2E** (`tests/e2e/*.spec.js`) — Playwright. Recorded MCP fixtures, real browser, golden-path flows.
- **Contract fixtures** (`tests/fixtures/mcp/<tool>.json`) — one per MCP tool, including a "future field" case and a "legacy" case.

## Provider setup

The UI supports Anthropic, OpenAI, OpenRouter, Ollama, and llama-server. Configure in the Settings page. Each provider has its own adapter under `src/lib/llm/providers/` with a common `streamChat({messages, tools}): AsyncIterable<Event>` surface. Adding a new provider means one adapter file plus a switch-case in the registry — no other file changes.

## Adding or changing a map provider

Map engines are intended to be pluggable: MapLibre, Mapbox, Leaflet, or Google Maps sit behind a shared `MapEngine` interface in `src/lib/map/`. Add a provider by creating `src/lib/map/providers/<name>/index.js` and registering it in `registry.js`; `MapCard.svelte` must never import a provider directly.

## Pull-request checklist

Before opening a PR:

1. `npm run check && npm run lint && npm run test && npm run build` — all pass.
2. No file introduced or grown past ~250 lines.
3. New MCP fields have a fixture case and a schema entry.
4. Diff contains no narrator comments and no dead code.

For behavior changes, include a screenshot or a short screen capture.

## Getting help

- Architecture and module map: [ARCHITECTURE.md](./ARCHITECTURE.md).
- MCP contract: [oba-mcp/docs/production/phase-0-baseline.md](../docs/production/phase-0-baseline.md) and [phase-6-operations.md](../docs/production/phase-6-operations.md).
- MCP tool reference: [`../mcp/CLAUDE.md`](../mcp/CLAUDE.md).
- Open a GitHub issue with the `ui` label for questions not covered above.
