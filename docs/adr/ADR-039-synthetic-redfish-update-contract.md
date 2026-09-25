# ADR-039 — Synthetic Redfish-shaped update mechanism

## Status

Accepted for FleetForge v1.

## Context

FleetForge needs a deterministic target boundary that can expose ambiguous
firmware-submission outcomes, task progression, reboot, inventory change,
health verification, and controlled observation faults. The portfolio scope
does not include external firmware images, specialized hardware, or an
external device emulator.

## Decision

The repository-owned Go synthetic target backend is the authoritative V1 target.
One process hosts many isolated targets and exposes a target-prefixed,
Redfish-shaped HTTP subset derived from a pinned DMTF schema release.

The supported update path is a bounded `application/octet-stream` POST to the
`HttpPushUri` advertised by each target's UpdateService. The request carries
explicit simulator-extension headers for the synthetic artifact SHA-256,
declared resulting version, and attempt correlation. The backend streams and
hashes the request body, never executes it, never retains the bytes, and
accepts only the version configured for the target scenario.

An accepted request returns `202 Accepted` with a target-scoped task URI unless
a configured post-accept response-loss fault closes the connection. Task,
reboot, inventory, health, event, and observation behavior advance only through
the deterministic scheduler. HTTP Basic authentication is sufficient for the
local V1 target boundary; public target credentials and the simulator admin
credential remain distinct and never enter scenarios or evidence.

## Correctness ordering

1. Validate target, authentication, headers, body bounds, and computed digest.
2. Append and sync `request_received` before response handling.
3. Apply any configured pre-accept fault.
4. Append and sync `update_accepted` before state mutation or response loss.
5. Reconstruct accepted state from the journal if the process stops before the
   in-memory engine applies the record.
6. Apply the post-accept response-loss fault or return the task reference.

The backend does not deduplicate update submissions. Two accepted requests
create two accepted-operation records and two task identities. FleetForge must
prevent or reconcile duplicates through its own persisted safety protocol.

## Supported resource families

- Service Root;
- Manager, ComputerSystem, and Chassis identity/status;
- UpdateService and one synthetic `HttpPushUri`;
- TaskService and Task;
- FirmwareInventory using SoftwareInventory fields;
- the EventService subset consumed by FleetForge;
- bounded Redfish-shaped errors and messages.

The supported-resource matrix freezes exact routes, fields, schema versions,
extensions, limits, and unsupported behavior before handlers are added.

## Evidence required

- pinned DMTF release and schema checksums;
- canonical request/response/error fixtures;
- scenario and normalized request-trace digests;
- deterministic replay report;
- independent request/acceptance journal;
- pre-accept and post-accept crash/recovery evidence;
- deliberate duplicate and false-absence oracle failures;
- 10, 100, and 1,000-target bounded-concurrency measurements; and
- an explicit statement that no external device compatibility was tested.

## Consequences

FleetForge can demonstrate its concurrency, coordination, ambiguity, recovery,
and evidence design locally and in CI. The contract is intentionally narrower
than the complete Redfish standard. Supporting an external device later would
require a separate adapter contract and independent evidence; it cannot change
or weaken the V1 safety invariants.
