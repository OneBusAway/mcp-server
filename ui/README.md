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
# 1. Start the MCP server (from the repo root or ../mcp/)
cd ../mcp && make serve-http

# 2. Install dependencies and start the dev server
npm install
npm run dev           # http://localhost:5173
```

Open the UI, go to **Settings**, set your MCP server URL and API key, then start chatting.

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| MCP Server URL | `http://localhost:8080` | URL of the running `mcp` HTTP server |
| Provider | Anthropic | AI provider: Anthropic, OpenAI, OpenRouter, Ollama, llama-server |
| API Key | — | API key for the selected provider |
| Model | `claude-haiku-4-5-20251001` | Model to use for chat |

Settings are stored in `localStorage` — no server-side config needed.

## Docker

Built and served from the root `docker-compose.yml`:

```sh
# From repo root
docker compose up -d oba-mcp-ui
```

Or build the image directly:

```sh
make docker-build
docker run -p 3000:3000 -e PUBLIC_MCP_URL=http://localhost:8080 oba-mcp-ui
```

## Project Layout

```
src/
  app.html               # HTML shell
  app.css                # Tailwind base styles
  lib/
    chat.svelte.js       # Global chat store (messages, streaming, history)
    mcp.js               # MCP HTTP client (callTool, listTools)
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
    api/chat/+server.js  # SSE streaming endpoint — bridges UI ↔ MCP ↔ AI
```

## Makefile

```sh
make dev            # start dev server (set MCP_URL=http://... to override)
make build          # production build
make preview        # preview production build
make docker-build   # build Docker image
make docker-up      # start via root docker-compose
make docker-logs    # follow container logs
```
