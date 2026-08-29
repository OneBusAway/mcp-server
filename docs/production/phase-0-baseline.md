# Production baseline

This document defines the operational contract for `oba-mcp` before production
hardening work begins. It is the source of truth for Phase 0 of the production
MCP plan.

## Deployment modes

| Mode | Intended use | Transport and exposure | Required configuration |
| --- | --- | --- | --- |
| Trusted local | A single developer's MCP client on the same machine. This is the default. | Stdio only; no listening network port. | `OBA_TRANSPORT=stdio` (or unset). The client owns process access and backend credentials. |
| Private HTTP | A trusted service-to-service integration in a private network. | HTTP is reachable only through private networking or an authenticated internal gateway. | Explicit `OBA_TRANSPORT=http`; gateway authentication, TLS, origin policy, and network restrictions. |
| Public HTTP | A shared, internet-facing MCP service. | Only an authentication and TLS gateway is public. The MCP server and Maglev remain private. | All Phase 1–5 release-gate requirements, caller authorization, rate limits, observability, and an approved operational owner. |

Stdio is the least-privileged default. HTTP must never be enabled merely to make
local development convenient; a deployment must opt in and select the matching
mode above. The current HTTP implementation is development-only until Phase 1
is complete.

## Tool profiles

The current server registers 29 tools. The `rider-default` profile is the
target default surface for conversational transit use. `advanced` tools remain
available only when a client explicitly requests that profile. `operator-only`
tools require an authorized operational client and must not be exposed through a
public rider profile. Profile enforcement is scheduled for Phase 4.

| Tool | Profile | Reason |
| --- | --- | --- |
| `get_agencies` | rider-default | Establishes available transit agencies. |
| `get_agency` | rider-default | Resolves an agency's basic rider-facing details. |
| `get_stop` | rider-default | Retrieves a known stop. |
| `search_stops` | rider-default | Resolves a stop from a rider's wording. |
| `find_stops_near_location` | rider-default | Finds nearby boarding points. |
| `get_stop_overview` | rider-default | Primary one-call stop and next-arrivals workflow. |
| `get_arrivals_for_stop` | rider-default | Primary known-stop arrival workflow. |
| `get_arrivals_for_location` | rider-default | Primary nearby-arrivals workflow. |
| `get_route` | rider-default | Retrieves a known route. |
| `search_routes` | rider-default | Resolves a route from name or number. |
| `get_routes_for_location` | rider-default | Finds service near a location. |
| `get_trip_for_vehicle` | rider-default | Answers a rider question about a known vehicle. |
| `get_trip_details` | rider-default | Gives live position/status for a known trip. |
| `get_current_time` | rider-default | Provides an API reference time for schedule questions. |
| `get_arrival_and_departure_for_stop` | advanced | Requires a trip ID and service date from another result. |
| `get_stops_for_agency` | advanced | Broad enumeration; useful for research, not typical rider chat. |
| `get_stop_schedule` | advanced | Potentially large full-day timetable. |
| `get_routes_for_agency` | advanced | Broad agency enumeration. |
| `get_stops_for_route` | advanced | Route exploration and itinerary context. |
| `get_trips_for_route` | advanced | Active-trip inspection for a route. |
| `get_trips_for_location` | advanced | Broad live-trip search. |
| `get_trip` | advanced | Static trip lookup by internal ID. |
| `get_shape` | advanced | Map/rendering geometry. |
| `get_stop_ids_for_agency` | operator-only | Raw ID bulk enumeration. |
| `get_route_ids_for_agency` | operator-only | Raw ID bulk enumeration. |
| `get_schedule_for_route` | operator-only | Detailed structure intended for integration/debugging. |
| `get_vehicles_for_agency` | operator-only | Fleet-wide real-time data. |
| `get_block` | operator-only | Internal operational block structure. |
| `get_metadata` | operator-only | Raw backend freshness and implementation metadata. |

## Compatibility and versioning policy

The MCP server uses semantic versioning. Before the first stable public release,
the version remains `0.x`; a public production launch starts at `1.0.0`.

- Patch releases fix defects and never intentionally change a published tool's
  name, required input, output field meaning, or error-code meaning.
- Minor releases may add optional inputs, optional output fields, new tools, or
  new profiles. Consumers must ignore unrecognized optional fields and tools.
- Major releases may remove a tool, make an optional input required, alter a
  field's meaning/type, or change a default in a way that changes results.
- A replacement tool is introduced first, while the old tool remains available
  for at least two minor releases and 90 days. Its description and response
  include the replacement and removal date.
- Tool names are immutable within a major version. Input and structured-output
  schemas are versioned with the server release and covered by contract tests.
- Text is a convenience summary, not a stable parsing contract. Phase 4 will
  publish `structuredContent` as the machine contract and a shared error
  envelope.

Every release notes tool/schema additions, changed defaults, deprecations, and
the earliest removal version/date. A client compatibility test is required for
every schema change.

## Service-level objectives

Measurements exclude client cancellations and requests rejected before upstream
work. Real-time tools are arrivals, vehicle, trip-status, and location queries;
static tools are agencies, routes, stops, shapes, and schedules. Measurements
are rolling 30-day production windows after Phase 6 metrics are available.

| Signal | Real-time target | Static target |
| --- | ---: | ---: |
| Successful tool-call availability | 99.5% | 99.9% |
| End-to-end p95 latency | 2.5 s | 1.0 s |
| End-to-end p99 latency | 5 s | 2.5 s |
| Public server-error rate | <0.5% | <0.1% |
| Upstream timeout rate | <1.0% | <0.25% |
| Memory-cache hit rate | >=40% | >=80% |

Availability is the proportion of valid, authorized calls that return a valid
success response. Expected empty results are successes. Upstream errors,
timeouts, malformed upstream responses, and internal failures count against
availability. Cache-hit targets guide capacity work; they are not availability
SLOs.

## Ownership and release decisions

Before any public HTTP launch, a named **service owner**, **security owner**,
and **on-call owner** must be recorded in the release issue or deployment
repository. The service owner approves tool-contract changes and SLO tradeoffs;
the security owner approves auth, authorization, secrets, and exposure changes;
the on-call owner approves alerts and runbooks. No public deployment is
permitted while any role is unassigned.

For local and private deployments, the deploying team owns credentials,
upstream capacity, and incident response. This repository owns the documented
default behavior and CI baseline below.
