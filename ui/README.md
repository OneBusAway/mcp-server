# onebusaway-mcp — UI

SvelteKit web chat interface for the [OneBusAway MCP server](../mcp/). Ask questions in plain English and get real-time transit data — arrivals, routes, stops, and live maps.

## Features

- AI chat backed by Claude (Anthropic), OpenAI, or any local model via Ollama / llama-server
- Live arrival boards auto-refreshed every 30 s
- Interactive maps (MapLibre GL) with stop markers and route polylines
- Route cards, stop suggestions, and multi-stop map views
- Dark mode, persistent chat history

## Requirements

- Node.js 20+
- `mcp` running in HTTP mode (`make serve-http` from `../mcp/`)
- An API key for Anthropic or OpenAI (set in the UI Settings page), or a local Ollama instance

## Quick Start

```sh
# 1. Start the MCP server (from ../mcp/)
cd ../mcp && OBA_HTTP_AUTH_TOKEN=local-dev make serve-http

# 2. Install dependencies and start the dev server
cd ../ui && npm install
MCP_AUTH_TOKEN=local-dev make dev # http://localhost:5173
```

The UI server holds MCP connection details. The browser never receives the
MCP bearer token or the upstream OBA API key.

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| Provider | Anthropic | AI provider: Anthropic, OpenAI, OpenRouter, Ollama, llama-server |
| API Key | — | API key for the selected provider |
| Model | `claude-haiku-4-5-20251001` | Model to use for chat |

Provider settings are stored in `localStorage`. Configure the UI server with
`OBA_MCP_URL` (the private MCP base URL) and `OBA_MCP_AUTH_TOKEN` (the private
MCP bearer token). These values must be set as deployment secrets, not browser
settings.

## Docker

Built and served from the root `docker-compose.yml`:

```sh
# From repo root
docker compose up -d oba-mcp-ui
```

Or build the image directly:

```sh
make docker-build
docker run -p 3000:3000 \
  -e OBA_MCP_URL=http://host.docker.internal:8080 \
  -e OBA_MCP_AUTH_TOKEN=local-dev oba-mcp-ui
```

## Project Layout

```
src/
  app.html               # HTML shell
  app.css                # Tailwind base styles
  lib/
    chat.svelte.js       # Global chat store (messages, streaming, history)
    mcp.js               # Same-origin MCP proxy client (callTool, listTools)
    settings.svelte.js   # Persistent UI settings (provider, model, API key)
    components/
      ArrivalsPanel.svelte  # Live arrival board for a stop
      ArrivalRow.svelte     # Single arrival row
      MapCard.svelte        # MapLibre map with stop/route markers
      BusLoader.svelte      # Loading spinner
      Icon.svelte           # Lucide icon wrapper
  routes/
    +layout.svelte       # App shell (sidebar + main)
    chat/+page.svelte    # Main chat page
    api/mcp/+server.js   # Server-only authenticated MCP proxy
    api/chat/+server.js  # SSE streaming endpoint — bridges UI ↔ MCP ↔ AI
```

## Makefile

```sh
MCP_AUTH_TOKEN=local-dev make dev # start dev server; MCP_URL overrides the target
make build          # production build
make preview        # preview production build
make docker-build   # build Docker image
make docker-up      # start via root docker-compose
make docker-logs    # follow container logs
```
