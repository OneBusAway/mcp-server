MCP_URL ?= http://localhost:8080

.PHONY: dev build preview install docker-build docker-up docker-down docker-logs

## install: install Node dependencies
install:
	npm install

## dev: start the SvelteKit dev server (requires oba-mcp running on MCP_URL)
dev:
	PUBLIC_MCP_URL=$(MCP_URL) npm run dev

## build: production build
build:
	npm run build

## preview: preview the production build locally
preview:
	npm run preview

## docker-build: build the Docker image
docker-build:
	docker build -t oba-mcp-ui .

## docker-up: start via the root docker-compose
docker-up:
	cd .. && docker compose up -d oba-mcp-ui

## docker-down: stop via the root docker-compose
docker-down:
	cd .. && docker compose down oba-mcp-ui

## docker-logs: follow container logs
docker-logs:
	cd .. && docker compose logs -f oba-mcp-ui

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
