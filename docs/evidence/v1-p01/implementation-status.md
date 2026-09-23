# Simulator implementation status

This is an evidence index for the repository-owned virtual-BMC contract.
Entries are complete only to the stated synthetic scope and must be revalidated
on the final integration SHA. It makes no external-device claim.

| Requirement | Current evidence | Status | Limitation / next gate |
| --- | --- | --- | --- |
| T01 resource matrix and fixture provenance | `supported-resource-matrix.md`, `fixture-provenance.md`, ADR-032/038/039 | Refreeze in progress | Pin exact DMTF schemas, checksums, fields, extensions, and canonical fixtures. |
| T02 versioned bounded scenarios | `internal/simulator/scenario`; strict-decoder and fixture tests | Implemented locally | Canonical post-update scenarios require the accepted update route. |
| T03 logical time and identities | `internal/simulator/clock`, `internal/simulator/core`; deterministic ordering tests | Implemented locally | Real-time demo adapter is not wired to a process. |
| T04 retained state and journal | `internal/simulator/journal`, `internal/platform/simulatorstate`; recovery/owner tests | Implemented locally | Process wiring and crash injection remain integration work. |
| T05 Redfish resources and authentication | None | Ready after Run 01 | Implement only the accepted synthetic routes, selected fields, extensions, and Basic-auth boundary. |
| T06 accepted-update ordering | Generic journal chain only | Ready after Run 03 | Add typed receipt/acceptance records and replay before accepting the synthetic route. |
| T07 update/event fault behavior | Fault catalog and strict name validation | Prepared | Implement against the accepted deterministic fault contract. |
| T08 separate admin boundary | `internal/application/simulator`, `internal/delivery/simulatoradmin`, listener-isolation tests | Partially implemented | Run lifecycle create/load/reset/advance/export and process configuration remain unwired. |
| T09 resource limits and readiness | Scenario bounds plus bounded listener handlers | Partially implemented | Runtime queue/connection/export enforcement and recovery readiness remain unwired. |
| T10 safe metrics | None | Not started | Requires the final command/event model to avoid emitting sensitive fields. |
| T11 canonical scenarios | One healthy scenario and invalid-input fixture | Partially implemented | Complete the accepted synthetic update/fault scenarios. |

## Cross-cutting proof already executable

- Scenario decoding rejects duplicate keys, unknown fields, trailing documents,
  unsupported faults, invalid bounds, and invalid topology.
- Logical timers have a defined total order and engine commands receive one
  serialized sequence.
- Journal recovery rejects a corrupt chain, sequence gap, hash mismatch, and
  leading snapshot; a retained run cannot have two owners.
- The public simulator boundary cannot reach the admin status endpoint.
- Architecture tests prohibit simulator imports of FleetForge persistence.

## Required before route-dependent work resumes

1. Pin the selected DMTF release, schema URLs/checksums, fixture license, and
   canonical fixture references in `fixture-provenance.md`.
2. Complete the supported-resource matrix with exact fields, routes,
   extensions, limits, errors, task semantics, inventory evidence,
   authentication, and recovery records.
3. Verify accepted ADR-032, ADR-038, and ADR-039 remain consistent with the
   fixtures before implementing route handlers.
