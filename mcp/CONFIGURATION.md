# Running and Configuring oba-mcp

This guide explains how to start `oba-mcp` with environment variables,
`.env`, `config.json`, Docker, and MCP clients.

Run local commands from the `mcp/` directory unless stated otherwise.

## Requirements

- Go 1.26.5 or newer (matching `go.mod`);
- Maglev or another OBA-compatible API;
- an API key accepted by that upstream server.

For the default local setup, Maglev is expected at:

```text
http://localhost:4000
```

## Configuration Precedence

The MCP resolves every source into one typed configuration:

```text
compiled defaults < config.json < .env < process environment
```

The value furthest to the right wins. Neither file is required.

Example:

```text
config.json       http.port = 8080
.env              OBA_PORT=9090
process variable  OBA_PORT=8081
final value       8081
```

Files are never discovered automatically. Select them explicitly with
`--config` and `--env-file`, or with `OBA_CONFIG_FILE` and `OBA_ENV_FILE`.

Configuration is startup-only and immutable. Changing `config.json`, `.env`,
or a process variable requires restarting the MCP server; hot reload is not
supported.

## Make Targets

| Command | Purpose |
|---|---|
| `make build` | Build `./oba-mcp` |
| `make run` | Run stdio with Make's local URL/API-key values |
| `make run-config CONFIG=... ENV_FILE=...` | Run using one or both configuration files |
| `make serve-http` | Run HTTP using environment/Make values |
| `make serve-http-config CONFIG=... ENV_FILE=...` | Force Streamable HTTP while loading files |
| `make check-config CONFIG=... ENV_FILE=...` | Validate the effective configuration and exit |
| `make print-config CONFIG=... ENV_FILE=...` | Print effective configuration with secrets redacted |
| `make mcp-add` | Register the environment-based stdio server with Claude Code |
| `make logs` | Follow the default log file |

`CONFIG` and `ENV_FILE` are optional for `check-config` and `print-config`, so
those targets can also validate an environment-only deployment.

## Option 1: Environment Variables Only

This is the existing workflow and remains the simplest production interface.

### Stdio

```sh
make run
```

The Make defaults are:

```text
OBA_BASE_URL=http://localhost:4000
OBA_API_KEY=test
OBA_TRANSPORT=stdio
```

Override them when needed:

```sh
make run \
  OBA_BASE_URL=https://oba.example.org \
  OBA_API_KEY=your-key
```

Stdio mode is normally launched by an MCP client. Starting it manually may
appear idle because it is waiting for MCP JSON-RPC messages on standard input.

### HTTP

HTTP mode requires a private bearer token:

```sh
OBA_HTTP_AUTH_TOKEN=local-mcp-token \
OBA_ALLOWED_ORIGINS=http://localhost:5173 \
make serve-http
```

Port `5173` is the default for `make dev-ui`. Use the exact origin of the
browser client making the request; the Docker Compose UI uses port `3000`.

The MCP endpoint is:

```text
http://127.0.0.1:8080/mcp
```

Clients must send:

```http
Authorization: Bearer local-mcp-token
```

`OBA_API_KEY` authenticates the MCP to Maglev. `OBA_HTTP_AUTH_TOKEN` provides
simple oba-mcp service authentication for an HTTP MCP client or private
gateway. It is not the complete MCP authorization model. They protect
different connections and should be different secrets in production. The
configuration remains additive so a future OAuth/OIDC mode can be introduced
without reusing `auth-token` for a different meaning.

MCP transport and MCP protocol version are separate concepts. `stdio` and
`streamable-http` control how messages move. Protocol-version negotiation is
handled by the MCP SDK and controls MCP behavior and compatibility.

## Option 2: Dotenv for Local Development

Create the local file:

```sh
cp .env.example .env
```

For stdio, a minimal `.env` is:

```dotenv
OBA_API_KEY=test
```

Start it with:

```sh
make run-config ENV_FILE=.env
```

For HTTP, use:

```dotenv
OBA_BASE_URL=http://localhost:4000
OBA_API_KEY=test
OBA_HTTP_AUTH_TOKEN=local-mcp-token
OBA_ALLOWED_ORIGINS=http://localhost:5173
```

Then start:

```sh
make serve-http-config ENV_FILE=.env
```

The real `.env` is ignored by Git. Do not commit it.

## Option 3: JSON Configuration

Create a local configuration:

```sh
cp config.example.json config.json
```

For an HTTP server, the important non-secret values are:

```json
{
  "$schema": "./config.schema.json",
  "upstream": {
    "base-url": "http://localhost:4000"
  },
  "mcp": {
    "transport": "streamable-http",
    "tool-profile": "all"
  },
  "http": {
    "bind-address": "127.0.0.1",
    "port": 8080,
    "allowed-origins": ["http://localhost:5173"]
  },
  "logging": {
    "path": "/tmp/oba-mcp.log",
    "format": "text"
  }
}
```

Supply secrets separately:

```sh
OBA_API_KEY=test \
OBA_HTTP_AUTH_TOKEN=local-mcp-token \
make run-config CONFIG=config.json
```

The real `config.json` is ignored by Git. The committed
`config.example.json` contains no credentials.

## Option 4: JSON Plus Dotenv

This is convenient for local development:

- `config.json` holds stable non-secret settings;
- `.env` holds credentials and personal overrides.

```sh
make check-config CONFIG=config.json ENV_FILE=.env
make print-config CONFIG=config.json ENV_FILE=.env
make run-config CONFIG=config.json ENV_FILE=.env
```

To guarantee Streamable HTTP mode regardless of the transport stored in the files:

```sh
make serve-http-config CONFIG=config.json ENV_FILE=.env
```

That target sets `OBA_TRANSPORT=streamable-http` as a process-level override.
The older value `http` remains accepted as a compatibility alias, but new
configuration should use the official transport name.

## Running the Binary Without Make

```sh
make build

./oba-mcp --config ./config.json --env-file ./.env
```

Environment selectors provide the same behavior:

```sh
OBA_CONFIG_FILE=./config.json \
OBA_ENV_FILE=./.env \
./oba-mcp
```

CLI selectors take precedence over `OBA_CONFIG_FILE` and `OBA_ENV_FILE`.

## Validate Before Starting

Validate every resolved requirement without opening the cache, log, upstream
connection, or MCP transport:

```sh
make check-config CONFIG=config.json ENV_FILE=.env
```

Successful output:

```text
configuration is valid
```

Valid configuration exits with status `0`. Invalid configuration exits
non-zero. In both cases the command exits without initializing logging, cache,
the OBA client, tools, or either MCP transport.

Inspect the final values safely:

```sh
make print-config CONFIG=config.json ENV_FILE=.env
```

Credentials appear as `[REDACTED]`. Do not add code that prints their real
values. Output also includes a `sources` object whose values are `default`,
`config.json`, `.env`, or `process-environment`.

`--print-config` still prints this redacted diagnostic when semantic
validation fails, then exits non-zero and writes the validation error to
stderr. A selected source file or environment value that cannot be parsed
prevents output.

## Claude Code and Other Stdio Clients

The existing environment-based registration remains available:

```sh
make mcp-add OBA_BASE_URL=http://localhost:4000 OBA_API_KEY=test
```

For file-based registration, configure the client command and arguments:

```json
{
  "command": "/absolute/path/to/oba-mcp",
  "args": [
    "--config",
    "/absolute/path/to/config.json",
    "--env-file",
    "/absolute/path/to/.env"
  ]
}
```

Use absolute paths because MCP clients may launch stdio servers from an
unexpected working directory.

## Docker Compose

The repository Compose configuration continues to use deployment environment
variables:

```sh
export OBA_API_KEY=test
export OBA_HTTP_AUTH_TOKEN=local-mcp-token
docker compose up -d oba-mcp
```

For JSON configuration, mount the file read-only and pass the selector as the
container command:

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

Kubernetes can use the same model: mount non-secret JSON from a ConfigMap and
inject credentials from a Secret. Explicitly selected symlinked files are
supported for Kubernetes-mounted configuration.

## Logs

The default log file is:

```text
/tmp/oba-mcp.log
```

Follow it with:

```sh
make logs
```

Set JSON logging with either:

```json
{
  "logging": {
    "format": "json"
  }
}
```

or:

```dotenv
OBA_LOG_FORMAT=json
```

`OBA_LOG_JSON=0|1` remains supported for compatibility. If the canonical and
legacy variables are set in the same source with conflicting meanings,
configuration loading fails.

Docker Compose forwards either logging variable only when the operator sets
it. Prefer `OBA_LOG_FORMAT`; `OBA_LOG_JSON` remains a one-release migration
path. Do not set both with conflicting values.

## Common Startup Errors

### `upstream.api-key is required`

Set `OBA_API_KEY`, add it to the selected `.env`, or add a local uncommitted
`upstream.api-key` value to `config.json`.

### `http.auth-token is required when mcp.transport is streamable-http`

HTTP transport has a separate authentication boundary. Set:

```dotenv
OBA_HTTP_AUTH_TOKEN=local-mcp-token
```

### Browser receives `403 Forbidden`

Add the exact browser origin, including scheme and port:

```dotenv
OBA_ALLOWED_ORIGINS=http://localhost:5173
```

Wildcards are intentionally rejected.

### Explicit configuration file cannot be loaded

Check the path supplied through `--config`, `--env-file`, `OBA_CONFIG_FILE`,
or `OBA_ENV_FILE`. The server intentionally fails instead of silently ignoring
a requested missing file.

### Cache or log filesystem failure

The server reports the failing path and exits when a configured log file or
non-empty persistent cache cannot be opened. Fix the directory, permissions,
or mounted volume. Use an empty cache path only when memory-only caching is
intentional.

### A JSON property is rejected

The loader rejects unknown properties, `null` values, multiple JSON documents,
and malformed types. Both `config.json` and `.env` have an exact 1 MiB maximum.
Use `config.schema.json` for editor completion and run `make check-config`
before deployment.

Parsing and validation failures are reported separately. For example,
`OBA_PORT=abc` is an environment parsing failure, while `OBA_PORT=99999` parses
successfully and then fails the valid port-range check.

### HTTP settings are rejected in stdio mode

Explicit `http.bind-address`, `http.port`, `http.auth-token`, or
`http.allowed-origins` settings are rejected when the selected transport is
`stdio`. Remove those settings or select `streamable-http`; this prevents an
operator from assuming unused security settings are active.

### Disabling the persistent cache

Set an explicitly empty cache path:

```dotenv
OBA_CACHE=
```

The process retains its bounded in-memory cache but does not open SQLite.

## Streamable HTTP Network Boundary

The default bind address remains `127.0.0.1`. `Origin` is checked against the
exact configured allow-list and wildcard origins are forbidden.

`0.0.0.0` is allowed only when explicitly configured. It makes the service
reachable on every container or host interface, so use it only behind the
documented private TLS/authentication gateway and network controls.

Streamable HTTP handles `SIGINT` and `SIGTERM` with a bounded graceful
shutdown. Stdio remains owned by the launching MCP client/process lifecycle.

## Security Summary

- Prefer environment or secret-manager injection for production credentials.
- Keep `config.json` and `.env` out of source control.
- Mount production JSON read-only.
- Never expose the HTTP MCP listener publicly without a TLS/authentication gateway.
- Never send the OBA API key to browsers or MCP clients.
- Never send the MCP HTTP bearer token to Maglev.
