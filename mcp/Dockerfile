# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

# GOTOOLCHAIN=local: use the compiler in this image even if go.mod requests a
# newer version. Update the FROM line when you upgrade your local Go toolchain.
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

ENV OBA_TRANSPORT=http
ENV OBA_PORT=8080
ENV OBA_BASE_URL=http://maglev:4000
ENV OBA_API_KEY=test
ENV OBA_LOG=/var/log/oba-mcp/oba-mcp.log
ENV OBA_CACHE=/var/cache/oba-mcp/cache.db

EXPOSE 8080

ENTRYPOINT ["./oba-mcp"]
