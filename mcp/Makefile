BINARY := oba-mcp
OBA_BASE_URL ?= http://localhost:4000
OBA_API_KEY  ?= test

OBA_PORT ?= 8080

.PHONY: build run watch test fmt lint clean mcp-add mcp-remove serve-http logs docker-build docker-up docker-down docker-logs

## build: compile the binary to ./oba-mcp
build:
	go build -o $(BINARY) .

## run: build and run the MCP server
run: build
	OBA_BASE_URL=$(OBA_BASE_URL) OBA_API_KEY=$(OBA_API_KEY) ./$(BINARY)

## watch: live-reload the server on file changes (via Air)
watch:
	OBA_BASE_URL=$(OBA_BASE_URL) OBA_API_KEY=$(OBA_API_KEY) air

## test: run all Go tests
test:
	go test ./...

## fmt: format all Go code
fmt:
	go fmt ./...

## lint: run go vet
lint:
	go vet ./...

## clean: remove build artifacts
clean:
	rm -f $(BINARY)
	rm -rf tmp/

## mcp-add: register this server with Claude CLI (user scope — available in all projects)
mcp-add: build
	-claude mcp remove oba-transit 2>/dev/null
	claude mcp add --scope user oba-transit $(PWD)/$(BINARY) \
		-e OBA_BASE_URL=$(OBA_BASE_URL) \
		-e OBA_API_KEY=$(OBA_API_KEY)

## mcp-add-log: register with logging to /tmp/oba-mcp.log (then: tail -f /tmp/oba-mcp.log)
mcp-add-log: build
	-claude mcp remove oba-transit 2>/dev/null
	claude mcp add --scope user oba-transit $(PWD)/$(BINARY) \
		-e OBA_BASE_URL=$(OBA_BASE_URL) \
		-e OBA_API_KEY=$(OBA_API_KEY) \
		-e OBA_LOG=/tmp/oba-mcp.log

## serve-http: run HTTP mode on loopback (requires OBA_HTTP_AUTH_TOKEN)
serve-http: build
	OBA_BASE_URL=$(OBA_BASE_URL) OBA_API_KEY=$(OBA_API_KEY) OBA_TRANSPORT=http OBA_PORT=$(OBA_PORT) ./$(BINARY)

## dev-ui: start the web UI dev server (run serve-http in another terminal first)
dev-ui:
	cd ../ui && npm run dev

## mcp-remove: unregister this server from Claude CLI
mcp-remove:
	claude mcp remove oba-transit

## logs: tail the log file with pretty output (Ctrl-C to stop)
logs:
	tail -f /tmp/oba-mcp.log

## docker-build: build the Docker image
docker-build:
	docker build -t oba-mcp .

## docker-up: start oba-mcp + maglev via Docker Compose
docker-up:
	docker compose up -d

## docker-down: stop and remove containers
docker-down:
	docker compose down

## docker-logs: follow logs from the running container
docker-logs:
	docker compose exec oba-mcp tail -f /var/log/oba-mcp/oba-mcp.log

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
