# Container deployment

This repository provides two intentionally separate Compose configurations:

- `docker-compose.yml` is for local development. It builds local source,
  binds the MCP and UI to loopback, and accepts development environment
  variables.
- `docker-compose.production.yml` is for a private production MCP service. It
  expects an immutable prebuilt image and Docker/Compose secret files. It does
  not publish an MCP port and does not deploy the UI.

The production topology is:

```text
authenticated TLS gateway → private oba-mcp → private Maglev/OBA API
```

Do not expose the Go MCP server directly to the internet. The gateway owns
public TLS, end-user authentication/authorization, request/IP rate limits, and
the public route to `/mcp`. The service bearer token remains a private
gateway-to-MCP credential.

## Local development

Create an uncommitted `.env` at the repository root with the required OBA API
key and HTTP bearer token, then run:

```sh
docker compose up --build
docker compose --profile ui up --build
```

The local MCP endpoint is bound to `127.0.0.1`, and the optional UI is also
loopback-only. This configuration is not a production deployment template.

## Private production MCP service

Build and publish an image using an immutable tag or, preferably, a digest:

```text
registry.example/onebusaway/oba-mcp@sha256:...
```

Store the two secret values in files controlled by the deployment platform:

```text
/run/operator-secrets/oba-api-key
/run/operator-secrets/oba-http-auth-token
```

Create an operator-only environment file, outside version control:

```dotenv
OBA_MCP_IMAGE=registry.example/onebusaway/oba-mcp@sha256:...
OBA_BASE_URL=http://maglev.internal:4000
OBA_ALLOWED_ORIGINS=https://transit.example
OBA_API_KEY_FILE=/run/operator-secrets/oba-api-key
OBA_HTTP_AUTH_TOKEN_FILE=/run/operator-secrets/oba-http-auth-token
```

Then start the service:

```sh
docker compose --env-file /secure/path/oba-mcp.env \
  -f docker-compose.production.yml up -d
```

The production container runs as UID/GID `10001`, drops Linux capabilities,
sets `no-new-privileges`, and has a read-only root filesystem. Only the named
cache/log volumes and a small `/tmp` tmpfs are writable. The readiness probe
uses the unauthenticated internal `/readyz` endpoint.

## What this configuration does not replace

- An authentication/authorization gateway and TLS termination.
- Gateway request/IP rate limiting with `429` and `Retry-After` behavior.
- An end-user session and CSRF/origin policy for the web UI.
- A secure provider-credential design for shared UI deployments.
- Image signing, vulnerability scanning, or an SBOM publication pipeline.

Those are separate release requirements. The UI is intentionally omitted from
the production Compose file until it satisfies its Phase U6 security work.
