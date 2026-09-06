# ADR-007 — One Go module with enforced inward package boundaries

- **Date:** 2026-09-06
- **Status:** Accepted
- **Owner phase:** 01

## Decision

FleetForge uses one Go module, `github.com/parth10-05/fleetforge`, with
independent `cmd/control-api`, `cmd/orchestrator-worker`, and
`cmd/redfish-simulator` binaries. Package dependencies flow inward:
delivery to application to domain. Platform, observability, persistence, and
Redfish implementations support inward-facing ports. The `internal/domain`
package cannot import HTTP, SQL, cloud, Redfish transport, or telemetry SDKs.

## Consequences

The single module keeps shared types and local builds simple during the early
monolith phase, while independent commands and containers preserve deployment
isolation. An architecture test protects the domain boundary; later adapters do
not receive permission to bypass it.

## Rollback

Separate modules require a replacement dependency/version strategy and an ADR
superseding this decision. They must not be introduced as a local workaround.
