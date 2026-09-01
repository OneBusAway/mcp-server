# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

# Keep container builds pinned to the toolchain supplied by the builder image.
ENV GOTOOLCHAIN=local

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# modernc.org/sqlite is pure Go — no CGO needed.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o oba-mcp .

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.20

# ca-certificates: outbound HTTPS to maglev/OBA API
# tzdata: time.LoadLocation() for agency timezones
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/oba-mcp .

# Logs land here inside the container; mount a volume to persist them.
RUN mkdir -p /var/log/oba-mcp

ENV OBA_LOG=/var/log/oba-mcp/oba-mcp.log
ENV OBA_CACHE=/var/cache/oba-mcp/cache.db

EXPOSE 8080

ENTRYPOINT ["./oba-mcp"]
