# Pre Phase 04 Virtual-BMC Contract Spike

## Outcome

Freeze the exact repository-owned virtual-BMC contract before the simulator
router and firmware adapter are implemented. This is a documentation,
fixture, and contract-test spike. It requires no cloud runtime, external image,
specialized hardware, or external device endpoint.

## Entry criteria

- Phase 03 persistence passes at its final integration SHA.
- ADR-014 and ADR-038 are accepted.
- The official DMTF release used as the source reference is pinned with schema
  checksums and retrieval date.
- The existing scenario, logical-clock, engine, journal, and admin-boundary
  preparation is available for review.

## Required questions

1. Which exact resource routes and fields does the FleetForge adapter consume?
2. Which fields are selected DMTF fields and which are explicit FleetForge
   simulator extensions?
3. What target-prefixed addressing scheme lets one process host many isolated
   virtual BMCs without exposing admin controls?
4. What bounded update request shape binds target, synthetic artifact digest,
   declared version, and attempt correlation?
5. Which task, reboot, inventory, health, and event observations are required
   for reconciliation?
6. Which pre-accept and post-accept boundaries must be journaled durably?
7. Which limits and error classes prevent unbounded bodies, queues, callbacks,
   connections, tasks, and journal growth?

## Micro tasks

- [ ] **VBCS-T01** Pin the selected DMTF release and checksums for every schema
  used by the supported subset.
- [ ] **VBCS-T02** Freeze target-prefixed Service Root, Manager, System,
  Chassis, UpdateService, TaskService, FirmwareInventory, EventService, and
  bounded error fixtures.
- [ ] **VBCS-T03** Freeze one `HttpPushUri`-style synthetic update request with
  byte, digest, version, authentication, deadline, and response limits.
- [ ] **VBCS-T04** Freeze deterministic task, reboot, inventory, health, event,
  contradiction, and task-disappearance timelines.
- [ ] **VBCS-T05** Freeze received-before-response and
  accepted-before-response-loss journal ordering plus restart replay.
- [ ] **VBCS-T06** Classify every field as selected-standard,
  simulator-extension, or admin-only and record unsupported operations.
- [ ] **VBCS-T07** Define Basic authentication for the local target boundary,
  a distinct admin credential, callback allowlisting, and redaction rules.
- [ ] **VBCS-T08** Accept ADR-032 and ADR-039 and update the resource matrix,
  fixture provenance, traceability, and risk records.

## Exit criteria

- Every supported route, field, extension, limit, state transition, fault, and
  error maps to a canonical fixture and an application port.
- The backend never executes or retains synthetic artifact bytes.
- Two accepted requests remain two accepted operations in the independent
  journal.
- Phase 04 can implement the contract without an external compatibility gate.
- The claim boundary explicitly excludes vendor, external-device, and complete
  Redfish compatibility.

## Stop conditions

- Stop if any route behavior remains implicit or has no fixture/test oracle.
- Stop if a proposed convenience would hide duplicate update submissions.
- Stop if a callback or returned URI can escape the configured target/test
  origin policy.
- Revise the ADR rather than inventing behavior in a handler.
