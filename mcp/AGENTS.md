# AGENTS.md — OneBusAway MCP

Canonical engineering rules for coding agents working on this Go MCP server.
Read this before changing code. `CLAUDE.md` provides repository and
OneBusAway-specific context.

```text
LLM ←MCP→ oba-mcp ←HTTP/cache→ Maglev or an OBA-compatible API ←GTFS/GTFS-RT
```

The MCP lives in `mcp/`; the optional SvelteKit UI is in `../ui/`. The MCP must
remain deployable without the UI. Consult the production MCP audit/TODO for
applicable planned hardening work.

## Change discipline

1. Read relevant code and tests before changing anything.
2. Understand the architecture and public MCP contract.
3. Follow an established production-ready pattern where one exists.
4. Make the smallest complete change for the requested task.
5. Do not refactor unrelated code or invent requirements.

## Architecture and contracts

Keep this boundary explicit:

```text
MCP tool → validation → client/service → OBA API → typed OBA DTO → MCP response type → structured MCP result
```

- Keep handlers thin. Put HTTP transport, caching, retries, circuit breaking,
  concurrency control, and upstream error classification in the client/service
  layer.
- Use named types for supported OBA and MCP contracts. Keep the upstream DTO
  and public MCP response separate, with an explicit transformation.
- Do not introduce `map[string]any` contracts for supported OBA endpoints,
  anonymous public response structs, or presentation-text parsing.
- Use `any` only at genuinely dynamic library boundaries; convert it to a
  named type promptly.
- Tool names, arguments, output schemas, and public error codes are public
  interfaces. Prefer additive, compatible changes.

## Go and context

- Write idiomatic, explicit Go; prefer simple focused functions and early
  returns to clever abstractions and deep nesting.
- Handle meaningful errors and wrap internal errors with `%w`.
- Comments explain why, not what obvious code already says.
- Do not leave dead code, debug code, commented-out implementations, or
  speculative abstractions.
- Avoid generic `utils`/`helpers`/`common` packages, one-use trivial helpers,
  forwarding wrappers, and interfaces without a concrete need.
- All request I/O must propagate the caller's `context.Context` through the
  handler, client/service, cache, and HTTP layers. Do not replace it with
  `context.Background()` in request processing.

## Input and upstream HTTP

- Treat every MCP argument as untrusted. Validate IDs, coordinates, dates,
  timestamps, radius, limits, windows, and free text before an upstream call.
- Coordinates must be finite: latitude `[-90, 90]`, longitude `[-180, 180]`.
- Do not rely solely on MCP schema validation, concatenate untrusted path
  segments, or let an argument select an arbitrary upstream host.
- Use safe path construction/escaping and `url.Values` for query parameters.
- Upstream requests must use the caller context, bounded timeouts, checked HTTP
  statuses, bounded response bodies, closed bodies, and consistent failure
  classification.
- Retry only safe idempotent operations on explicitly transient failures. Use
  bounded retry budgets, backoff with jitter, and honor `Retry-After`.

## Reliability and results

- Put retries, caching, concurrency limits, request coalescing, and circuit
  breaking in the shared client layer, never individual handlers.
- Bound concurrency and cache size. Cache only validated successful data; never
  include raw credentials or secrets in cache keys.
- Every goroutine needs a clear purpose, lifetime, and termination condition.
- Circuit breakers count upstream-health failures, not invalid IDs or other
  normal application errors. An open circuit fails fast with a stable,
  retryable MCP error.
- Structured MCP content is the canonical machine-readable result. Text may be
  supplementary, never required for parsing data.
- Preserve IDs, epoch milliseconds, timezones, and realtime/prediction state.
  Bound lists and report truncation.

## Errors and security

- Internal errors are not public MCP errors. Never expose stack traces, paths,
  URLs, secrets, raw authentication details, SQL errors, or raw `err.Error()`.
- Translate expected failures into stable public MCP error codes and state
  whether retrying may succeed.
- Return expected tool failures as MCP tool results when SDK-level errors could
  close or destabilize the connection.
- Production HTTP topology is: authenticated TLS gateway → private MCP service
  → private OBA service. Do not weaken CORS, host protection, authentication,
  or network boundaries for convenience.
- Never hard-code, commit, log, or return credentials. Tool arguments must not
  select upstream hosts.

## OBA domain

- OBA timestamps are Unix milliseconds; use `time.UnixMilli` for display.
- Use the agency timezone for human-readable transit times where applicable.
- Decode the OBA envelope and `references` into named DTOs; explicitly map
  needed shared stops, routes, trips, and agencies into MCP response types.

## Tests and completion

- Behavior changes require tests. Prefer table-driven validation tests,
  deterministic OBA fixtures, `httptest` client tests, and named-response
  contract tests.
- Cover success, empty, invalid input, malformed upstream data, HTTP failures,
  timeouts/cancellation, response limits, retries, circuit state, caching, and
  other relevant failure paths. Do not depend on live transit feeds.
- Do not manually edit sqlc-generated files under `cachedb/`.
- Before finishing, run:

  ```sh
  make fmt
  make lint
  make test
  ```

- Review the complete diff for unintended contract changes, unvalidated input,
  unbounded output, secret exposure, missing context propagation, unnecessary
  abstractions, and unrelated changes. Report checks that could not run.
