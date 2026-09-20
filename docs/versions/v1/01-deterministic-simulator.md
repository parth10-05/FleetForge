# V1-P01 — Deterministic Redfish Simulator

## Goal

Implement the narrow Redfish subset frozen by V1-P00 with persistent seeded scenarios and an independent acceptance journal. The simulator must expose repeated firmware requests; it must never hide a controller defect by deduplicating requests.

## Tasks

- [ ] **V1-P01-T01** Record the exact supported DMTF/OpenBMC resource subset, schema references, fixture provenance, and simulator-only control-plane boundary.
- [ ] **V1-P01-T02** Define a versioned scenario format containing seed, 6–10 targets, at least two racks, initial versions, capabilities, timing, and a bounded fault schedule.
- [ ] **V1-P01-T03** Implement injected logical time for tests, a real-time demo adapter, deterministic target identity derivation, and input validation with bounded target/fault counts.
- [ ] **V1-P01-T04** Persist per-run target state and an immutable append-only request/acceptance journal. Enforce one active process owner per retained run and fail closed on unrecoverable recovery.
- [ ] **V1-P01-T05** Implement service root, target inventory/capabilities, UpdateService action, TaskService, SoftwareInventory, and the minimal session/auth surface actually consumed by the v1 adapter.
- [ ] **V1-P01-T06** Record every received update request before response handling and every accepted operation before injecting response loss. In the non-idempotent profile, two requests must remain two visible accepts.
- [ ] **V1-P01-T07** Implement only the v1 fault profiles: pre-write connection failure, post-accept disconnect, lost/duplicate/out-of-order event, task disappearance after reboot, version contradiction, health failure, and controlled `429/503` observation errors.
- [ ] **V1-P01-T08** Isolate admin scenario/reset/journal endpoints from the Redfish listener and protect them with a separate admin path/secret. Redfish responses may not disclose simulator controls.
- [ ] **V1-P01-T09** Bound response bodies, headers, request methods, callback workers, open connections, journal growth, and fault rates; expose simulator readiness only after recovery and journal checks.
- [ ] **V1-P01-T10** Emit safe request/task/fault metrics and correlation values without credentials or firmware bytes.
- [ ] **V1-P01-T11** Create canonical healthy, lost-event, post-accept, task-disappearance, and canary-failure scenarios for later v1 phases.

## Required proof

- Same seed, scenario, and request sequence yields a normalized identical journal/result.
- Post-accept loss creates exactly one accepted operation; an intentional duplicate creates two and fails the independent oracle.
- A restart preserves run state/journal or marks the run invalid.
- Unsupported resource/action and public simulator-control access cannot mutate state.

## Exit gate

Every v1 fault can be reproduced from a saved seed and the simulator can independently prove when FleetForge sent duplicate update requests. Do not claim full Redfish or OpenBMC compliance.
