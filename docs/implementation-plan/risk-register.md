# Risk Register

## Scale

- Likelihood: 1 rare, 2 unlikely, 3 possible, 4 likely, 5 almost certain.
- Impact: 1 negligible, 2 minor, 3 material, 4 severe, 5 project/safety critical.
- Score: likelihood × impact. Scores 15–25 require explicit mitigation before the affected phase gate; 8–14 require active tracking; 1–7 are monitored.

## Initial register

| ID | Risk | L | I | Score | Trigger/evidence | Mitigation/contingency | Primary phase | Status |
|---|---|---:|---:|---:|---|---|---:|---|
| R-01 | Update accepted or submitted by a stale live worker, causing duplicate submission | 4 | 5 | 20 | Post-accept disconnect, worker stall beyond lease, or late request | Reconciliation-only expired submit, late-caller exclusion, evidence hierarchy, independent accepted-operation journal | 09, 14 | Open |
| R-02 | Simulator routes or payloads drift from the accepted synthetic contract | 3 | 4 | 12 | Fixture/schema diff or adapter contract failure | Pinned schema checksums, canonical fixtures, contract tests, explicit extensions | Contract freeze, 04, 14 | Open |
| R-03 | Pause is described as a network fence although an already admitted worker can write later | 3 | 5 | 15 | Barrier after admission and before request write | Persist epoch-fenced admission; block new admission; expose earlier admission as potentially in flight; remove stronger claim | 02, 09 | Open |
| R-04 | Retry storm amplifies target/network/database failure | 4 | 4 | 16 | Correlated 429/503, queue backlog, pool saturation | Jitter, multi-scope budgets, queue/app caps, circuit protection, auto-pause | 08, 09, 14 | Open |
| R-05 | Database connection exhaustion under ECS Fargate autoscaling | 4 | 4 | 16 | Pool wait/timeouts/Neon limit | Pooled endpoint, conservative pools, max instances/concurrency, metrics/load test | 03, 13, 14 | Open |
| R-06 | SQS duplicate receipt, visibility expiry, or uncertain publish prevents durable work repair | 3 | 5 | 15 | Repeated/lost message or unknown send result leaves due work overdue | PostgreSQL due work, semantic command ID, delivery generation, SQS receipt/publish tests, scheduled sweeper | 03, 08 | Open |
| R-07 | Target endpoint enables SSRF/internal network access | 3 | 5 | 15 | Disallowed DNS/IP/redirect accepted | Canonical validation, CIDR/port allowlist, DNS recheck, no redirect, egress controls | 05, 13 | Open |
| R-08 | Firmware object replaced after verification | 3 | 5 | 15 | Same key/new generation used | Bind generation+digest, immutable object policy, campaign snapshot | 06 | Open |
| R-09 | Credential/signed URL/firmware data leaks into telemetry or UI | 3 | 5 | 15 | Canary secret found | Direct storage, allowlists, redaction, leakage tests | 06, 10–12 | Open |
| R-10 | Campaign counter contention or cross-writer deadlock at 1,000 targets | 3 | 3 | 9 | Lock wait, abort/completion/evaluator deadlock, or domain starvation | One complete lock order, short transactions, barrier tests; stripe/project only if measurements demand | 03, 07, 08, 14 | Open |
| R-11 | Lost/out-of-order events regress or stall workflow | 4 | 4 | 16 | State regression/overdue work | Events as hints, monotonic reducer, polling/sweeper | 04, 09, 14 | Open |
| R-12 | Task disappears across reboot and outcome misclassified | 3 | 5 | 15 | Missing task with changed version | Inventory/activation/health evidence hierarchy, conservative attention | 04, 09, 14 | Open |
| R-13 | Schema migration prevents application rollback | 3 | 4 | 12 | Old revision fails after migration | Expand/migrate/contract, compatibility CI, rollout rehearsal | 03, 13 | Open |
| R-14 | Periodic sweeps prevent Neon scale-to-zero or free-tier/pricing changes block services | 4 | 3 | 12 | CU-hours, storage, telemetry, or capacity exceeds budget | Session-based free environments, paid retained-demo budget, reverify prices, provider seams, short-lived tasks | 13, 14 | Open |
| R-15 | Local/CI capacity is insufficient for the 1,000-target evidence run | 3 | 3 | 9 | Load run exceeds recorded memory, file, or time budget | Compact target state, bounded queues/journal, measured runner profile, smaller PR smoke | 04, 14 | Open |
| R-16 | Single-owner engine or journal fsync masks controller parallel speedup | 4 | 3 | 12 | Throughput flattens while worker concurrency rises | Measure queue/fsync latency, preserve correctness, report bottleneck before tuning | 04, 14 | Open |
| R-17 | Telemetry cardinality/retention exceeds free budget | 4 | 3 | 12 | Ingestion/usage warning | Bounded labels, IDs in traces/logs, sampling/retention/cost alerts | 12, 13 | Open |
| R-18 | Operator UI hides stale/ambiguous/in-flight reality | 3 | 4 | 12 | User misinterprets state | Freshness/revision/evidence UX, explicit attention/abort caveat, E2E usability review | 11 | Open |
| R-19 | Scope grows into Kubernetes/Kafka/vendor fleet before core proof | 4 | 4 | 16 | New dependencies without failed requirement | Phase gates, ADR, non-goals, evidence-first alternatives | All | Open |
| R-20 | Project becomes CRUD/UI-heavy and systems core is delayed | 3 | 4 | 12 | Screens built before fault core | Phase order prioritizes domain/persistence/simulator/orchestration | All | Open |
| R-21 | Test flakiness hides concurrency defects | 3 | 4 | 12 | Reruns needed/unstable seeds | Barrier-controlled races, seed retention, no safety-test quarantine | 02, 08, 09, 14 | Open |
| R-22 | Benchmark results are irreproducible or overstated | 3 | 4 | 12 | Missing commit/env/seed/raw data | Evidence schema, immutable reports, claim traceability | 14, 15 | Open |
| R-23 | Simulator admin/fault controls become reachable through the target listener | 2 | 5 | 10 | Public route or credential accesses reset/advance/export | Separate listener/credential, routing tests, loopback default, deny-by-default deployment | 04, 13, 14 | Open |
| R-24 | Auth provider/session design weakens API security | 3 | 4 | 12 | Token storage/claim limitations | OIDC/PKCE, audience/issuer checks, recent-auth policy, server enforcement | 10, 11 | Open |
| R-25 | Infrastructure state or CI token grants excessive privilege | 3 | 5 | 15 | Untrusted PR/IAM test succeeds | GitHub OIDC trust-condition restriction, least privilege, state security, IAM test | 13 | Open |
| R-26 | Work lease expiry or `NEEDS_ATTENTION` releases capacity while a target operation is still active | 4 | 5 | 20 | Simulator reports physical operations above configured rack/site/global maximum | Durable operation reservation independent of lease; attention retains quarantine/capacity until safe release | 08, 09, 14 | Open |
| R-27 | Zero ECS Fargate worker capacity has no external wakeup for unpublished/due work | 4 | 4 | 16 | Database work remains overdue with no worker task running | EventBridge Scheduler launches a bounded publisher/sweeper task; alarm on repeated failures | 08, 12, 13 | Open |
| R-28 | Synthetic target restart loses or forks target truth | 4 | 5 | 20 | Accepted operation disappears or divergent journals exist | Single-owner runtime, durable run state/journal, restart recovery tests, explicit run invalidation when recovery cannot prove continuity | 04, 14 | Open |
| R-29 | Synthetic artifact streaming, digest binding, or task semantics drift between simulator and worker | 3 | 5 | 15 | End-to-end contract test disagrees on body, headers, acceptance, or task identity | Contract freeze contract freeze, shared fixtures, bounded integration tests, ADR revision on change | Contract freeze, 04, 09 | Open |
| R-30 | Restore replays older DB state against a device that already advanced | 3 | 5 | 15 | Restored due work attempts a duplicate submission | Recovery mode disables submission; fresh delivery generations; reconcile newer persistent device state first | 03, 09, 14 | Open |
| R-31 | Artifact lifecycle deletes a verified object still referenced by a campaign | 3 | 5 | 15 | Age-based bucket rule races with reference acquisition | Temporary-object TTL only; database-aware exact-generation deletion; holds/retention for referenced artifacts | 06, 13 | Open |
| R-32 | Simulator-side semantic dedupe hides a FleetForge duplicate submission | 3 | 5 | 15 | Deliberate double-submit test does not fail the oracle | Independent received-request and accepted-operation journal; non-idempotent target profile; false-proof-of-absence test | 04, 09, 14 | Open |

## Review cadence

- Review before each phase starts and at every phase exit gate.
- Re-score after a fault, load, parallel-efficiency, security, cost, or restore result.
- Add discovered risks with evidence and owner.
- Close only when proof shows the risk is removed; mark accepted when mitigation leaves understood residual risk.
- Any score 20–25 stops affected implementation until mitigated/approved.
- An accepted service decision does not close a risk; only the listed executable evidence closes it.

## Required closure evidence

- Safety/concurrency risk: deterministic test and retained seed/trace.
- Security risk: negative test/threat review and configuration evidence.
- Cost risk: measured usage and enforced cap/alert.
- Provider risk: verified fallback or explicit accepted dependency.
- Portfolio risk: claim-to-evidence mapping and external review.
