# Simulator implementation status

This is an evidence index, not a claim of QEMU/OpenBMC compatibility. Entries
are complete only to the stated local-contract scope and must be revalidated on
the final integration SHA.

| Requirement | Current evidence | Status | Limitation / next gate |
| --- | --- | --- | --- |
| T01 resource matrix and fixture provenance | `supported-resource-matrix.md`, `fixture-provenance.md`, ADR-038/039 | Prepared | Exact fields and schemas require the selected QEMU fixture. |
| T02 versioned bounded scenarios | `internal/simulator/scenario`; strict-decoder and fixture tests | Implemented locally | Canonical post-update scenarios require the accepted update route. |
| T03 logical time and identities | `internal/simulator/clock`, `internal/simulator/core`; deterministic ordering tests | Implemented locally | Real-time demo adapter is not wired to a process. |
| T04 retained state and journal | `internal/simulator/journal`, `internal/platform/simulatorstate`; recovery/owner tests | Implemented locally | Process wiring and crash injection remain integration work. |
| T05 Redfish resources and session/auth | None | Blocked | ADR-039 and pinned capability fixture must freeze route, payload, task, inventory, and auth details. |
| T06 accepted-update ordering | Generic journal chain only | Blocked | No update request may be accepted until the observed route exists. |
| T07 update/event fault behavior | Fault catalog and strict name validation | Prepared | Event and transport semantics require the fixture-backed contract. |
| T08 separate admin boundary | `internal/application/simulator`, `internal/delivery/simulatoradmin`, listener-isolation tests | Partially implemented | Run lifecycle create/load/reset/advance/export and process configuration remain unwired. |
| T09 resource limits and readiness | Scenario bounds plus bounded listener handlers | Partially implemented | Runtime queue/connection/export enforcement and recovery readiness remain unwired. |
| T10 safe metrics | None | Not started | Requires the final command/event model to avoid emitting sensitive fields. |
| T11 canonical scenarios | One healthy scenario and invalid-input fixture | Partially implemented | Fault/update scenarios require observed semantics. |

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

1. Populate the selected image source, digest, checksum, license, version, and
   redacted fixture references in `fixture-provenance.md`.
2. Accept ADR-039 with one observed service root, update mechanism, request
   representation, response/task semantics, inventory evidence, auth/trust
   procedure, and recovery record.
3. Update ADR-032 from proposed only after the reversible QEMU/OpenBMC test
   image update and recovery record are reviewed.
