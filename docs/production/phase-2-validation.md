# Phase 2 — Input validation contract

Every parameterized MCP tool validates its arguments before it contacts a
OneBusAway API server. Shared rules live in `mcp/validation`; handler adapters
live in `mcp/tools/input.go`.

## Rules

- Entity IDs are trimmed, limited to 256 characters, and may contain only
  letters, digits, `_`, `-`, `.`, and `:`. Path separators, query syntax,
  percent encoding, control characters, and `.`/`..` are rejected.
- IDs are passed to typed client methods, which escape URL path segments.
- Latitude is finite and within `[-90, 90]`; longitude is finite and within
  `[-180, 180]`.
- Radii are whole metres. Location tools default to 500 m and allow 1–5,000 m.
- Search text is trimmed, non-empty, and at most 200 characters.
- Search result limits are whole numbers from 1–20.
- Arrival windows are whole minutes from 0–120. Defaults are documented by
  each tool.
- Dates are strict `YYYY-MM-DD` service dates interpreted by the relevant
  agency backend.
- Timestamps are whole Unix milliseconds between 1 and 2100-01-01 UTC.
- Optional booleans, strings, and numbers reject an unexpected JSON type.

Validation failures return an MCP tool error and do not make an upstream call.
The validation contract tests cover invalid IDs, encoded separators, malformed
dates, invalid coordinate/radius/span bounds, fractional timestamps and
limits, and unexpected argument types across all parameterized handlers.
