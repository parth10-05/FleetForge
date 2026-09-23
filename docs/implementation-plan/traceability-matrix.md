# Architecture-to-Implementation Traceability Matrix

## Purpose

This matrix ensures that every architecture invariant and major non-functional requirement has an implementation owner, executable proof, and evidence artifact. Update it when phases or contracts change.

## Safety invariants

| ID | Requirement | Primary phase | Enforcement | Required test/evidence |
|---|---|---:|---|---|
| INV-01 | At most one non-terminal campaign per target | 03, 05, 07 | Partial unique constraint + readiness guard | P03-IT-09, P05-IT-05, P07-IT-01 |
| INV-02 | Persist intent/attempt before firmware side effect | 03, 09 | Transactional step intent and instrumented submission boundary | P09-FT-02/03 plus trace |
| INV-03 | Never blindly or belatedly resubmit an uncertain update | 04, 09, 14 | Reconciliation-only expired submit, evidence hierarchy, late-caller exclusion | P09-FT-02/04/05/15/16/17; independent accepted-operation journal |
| INV-04 | No later wave before prior wave terminal/evaluated | 02, 07 | Domain guard, wave revision, transactional release | P02-PT-03, P07-IT-03 |
| INV-05 | Pause/abort commit blocks later-granted submission authorization | 02, 08, 09 | Transactional admission against command epoch; earlier admissions remain potentially in flight | P02-PT-06, P09-FT-13/18 |
| INV-06 | Policy/artifact/target snapshot immutable after readiness | 03, 05, 06, 07, 10 | Frozen snapshot/revision/DB/application guards | Negative API/DB tests, snapshot hash |
| INV-07 | Every lifecycle mutation has same-transaction audit | 03, 10 | Unit-of-work writes aggregate + audit | P03-IT-02/03, P10-AT-01 |
| INV-08 | Queue/event transport is not workflow truth | 03, 08, 09 | Due work/outbox + state reload + sweeper | P08-FT-06/07, delete/rebuild demonstration |
| INV-09 | Success requires expected version and health | 02, 04, 09 | Verification reducer and contradictory fixtures | P04-FT-05, P09-FT-10/11 |
| INV-10 | Secrets/firmware bytes excluded from telemetry/browser/events | 01, 06, 10, 11, 12 | Direct storage, allowlists, redaction | P06/P10 security tests, P12-LG-01 |
| INV-11 | No DB lock held during network call | 03, 05, 08, 09 | Claim-commit-call-result protocol | Instrumented blocking-adapter integration test |
| INV-12 | Retries are finite, multi-scope budgeted, and resume the correct step | 02, 08, 09, 14 | Attempt/deadline/domain counters + jitter + explicit resume step | P02-PT-05, P08-RT-01, P09-FT-12/19, L5 report |
| INV-13 | Physical operation concurrency survives handler lease expiry and ambiguity | 08, 09, 14 | Durable operation reservation independent of work lease | P08-CT-03, P09-FT-17, L3/L5 active-operation series |
| INV-14 | Target ownership survives target-update terminality and unresolved operations | 03, 05, 07, 09 | Readiness enrollment claim and unresolved-operation quarantine | P03-IT-15, P05-IT-05, P09-FT-20 |
| INV-15 | Simulator acceptance truth is independent, durable, and never hides duplicate requests | 04, 09, 14 | Seeded scenario, single-owner engine, append-only acceptance journal, and independent oracle | V1 simulator journal/oracle mutation tests; P14-T21/T21A/T21B |

## Functional requirements

| ID | Requirement | Phase(s) | Proof |
|---|---|---:|---|
| FR-01 | Register and discover targets through Redfish | 04, 05, 10, 11 | Simulator contract, discovery integration, API/UI E2E |
| FR-02 | Upload/reference and verify immutable artifact | 06, 10, 11 | Object/digest/signature negative tests and upload E2E |
| FR-03 | Create/validate/freeze campaign | 05, 07, 10, 11 | Snapshot hash, plan golden tests, UI E2E |
| FR-04 | Select canary and ordered waves | 07 | Determinism/property/golden plan |
| FR-05 | Enforce physical-operation concurrency by global/site/rack | 08, 09, 14 | Reservation race plus simulator active-operation time series across expiry/attention |
| FR-06 | Submit/update/observe/reboot/verify target | 09, 14 | Full simulator flow + QEMU record |
| FR-07 | Pause/resume/abort safely | 02, 07–11 | Epoch race, E2E command and UI tests |
| FR-08 | Reconcile lost events/tasks | 04, 08, 09, 14 | Lost-event/task fault reports |
| FR-09 | Handle manual attention | 09–11 | Admin/recent-auth/audit E2E |
| FR-10 | Reconstruct audit and replay decisions | 02, 03, 10, 14 | Replay equality and restore reconstruction |

## Non-functional requirements

| ID | Requirement | Phase(s) | Target/evidence |
|---|---|---:|---|
| NFR-01 | 1,000 simulated targets without redesign | 04, 07–09, 11, 14 | L2–L5 load reports |
| NFR-02 | Cloud-first; no laptop DB/heavy OpenBMC build | 01, 13, 14 | Developer setup and remote QEMU/builder record |
| NFR-03 | API p95 design target under 500 ms for non-device endpoints | 10, 12, 14 | SLO-01 measured profile |
| NFR-04 | 99% runnable due work starts within 60 s design target | 08, 12, 14 | SLO-02 measurement |
| NFR-05 | Zero blind or stale-worker duplicate submission in fault suite | 09, 14 | Independent SLO-03 accepted-operation and proven-absence report |
| NFR-06 | 100% mutation audit completeness in integration tests | 03, 10, 12 | SLO-04 query/report |
| NFR-07 | Actionable reconciliation/outbox alerts | 12–14 | Alert injection/runbook drills |
| NFR-08 | Secure identity/RBAC/SSRF/TLS/secrets | 05, 06, 08, 10, 13 | Threat tests and IAM matrix |
| NFR-09 | Reproducible CI/CD and rollback | 01, 03, 13 | Clean runner, same digest, rollback drill |
| NFR-10 | Isolated restore and decision replay | 03, 12–14 | Restore/integrity/replay report |
| NFR-11 | Bounded cloud cost/free-tier awareness | 06, 12–15 | Cost ledger, caps, teardown, observed run cost |
| NFR-12 | Honest QEMU/OpenBMC compatibility evidence | 04, 05, 09, 14, 15 | Redacted fixture and integration report |

## Architecture component coverage

| Component | Build phase | Hardening/validation phase |
|---|---:|---:|
| Control API | 01 baseline, 10 complete | 12–14 |
| Domain reducers/policies | 02, 07 | 09, 14 |
| PostgreSQL/outbox/due work/enrollment | 03 | 08, 12–14 |
| Redfish simulator on persistent VM | Pre-04 capability spike, 04 | 09, 14 |
| Redfish adapter/inventory | 05 | 09, 14 |
| Artifact service | 06 | 10, 13, 14 |
| Orchestrator worker | 08, 09 | 12–14 |
| Operator console | 11 | 12–15 |
| Telemetry/runbooks | incremental, completed 12 | 13–15 |
| Cloud infrastructure/CI/CD | 01 baseline, 13 complete | 14–15 |

## Updating this matrix

- A requirement may have multiple phases but exactly one primary implementation owner.
- A test identifier must resolve to a phase/test specification or repository test manifest.
- A requirement cannot be marked complete without an evidence location.
- If a requirement is deferred, record scope/reason/risk in the Decision Log and Risk Register.
- If an invariant is weakened, architecture approval is required before implementation.

## Simulator preparation status

The deterministic simulator's logical-time/scenario boundary is accepted in
ADR-014 and isolation/single-owner preparation is recorded in ADR-038. The
QEMU/OpenBMC update contract remains blocked until ADR-013 and ADR-032 have
accepted evidence. Therefore this matrix does not treat simulator preparation
as proof of P04-LT-01, P04-LT-02, full Phase 04 completion, Redfish/OpenBMC
compatibility, or a 1,000-target measurement.
