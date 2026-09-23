# FleetForge Architecture Freeze

## Status and authority

This document records the service and correctness decisions approved after the pre-freeze architecture review. It is normative together with the System Architecture RFC. If an older sentence in a phase document conflicts with this record, implementation stops until that phase document and the RFC are corrected. The Decision Log contains the corresponding ADR status.

## Frozen service map

| Need | Frozen choice | Scope and deciding rule |
|---|---|---|
| Control API and task handlers | Amazon ECS services on AWS Fargate | Request-driven, stateless roles with minimum instances zero, bounded maximum instances, and connection/concurrency budgets |
| Deterministic virtual-BMC backend | Local/containerized Go simulator role | One process hosts many isolated targets; one stable owner controls mutable state, logical timers, callbacks, and the independent accepted-operation journal |
| Workflow database | Neon PostgreSQL | Pooled endpoint for application roles; direct endpoint for migrations and transaction-level/session-sensitive administration; Free only for disposable sessions and Launch with a seven-day restore window for retained demo state |
| Delayed work | Amazon SQS Standard | At-least-once queue wakeups; PostgreSQL remains authoritative, and SQS message IDs or delivery counts are never correctness mechanisms |
| Repair wakeup | Amazon EventBridge Scheduler | Launches an IAM-authorized bounded publisher/sweeper ECS task; this is the external clock that recovers durable work when worker capacity is zero |
| Event fan-out | No event broker in the first portfolio release | No required independent consumer justifies a second asynchronous delivery system; SSE uses database-backed projections and telemetry exports directly through OpenTelemetry |
| Firmware objects | Amazon S3 with Versioning | Exact bucket, object key, version ID, size, and SHA-256 bind artifact identity; verified-object deletion is database-aware rather than age-only |
| Human identity | Amazon Cognito User Pools with Google federation | OIDC identity adapter, secure server-managed session or in-memory token flow, explicit `auth_time` recent-auth checks, and application-owned RBAC |
| Telemetry | OpenTelemetry to Grafana Cloud Free | One destination, bounded labels, sampling/retention budgets, and retained release evidence outside the live backend |
| Runtime secrets | AWS Secrets Manager | Worker-task-role least privilege for BMC credentials; a separate administrative CLI provisions and rotates values without passing them through FleetForge API, browser, database, GitHub Actions, or Terraform state |
| CI deployment identity | GitHub Actions OIDC to AWS IAM | Short-lived credentials restricted by immutable repository/owner identity plus protected ref, workflow, and environment claims |
| Device test boundary | Repository-owned Redfish-shaped virtual-BMC contract | Exact selected DMTF fields and simulator extensions are frozen in ADR-032/039; no external-device compatibility claim is part of the release |

## Correctness contract

### Submission admission and pause

`command_epoch` is an admission fence, not an external network fence. A worker transaction may grant one persisted submission authorization only while the captured epoch and all safety gates are current. A pause or abort commit prevents new authorizations. An authorization granted before that commit is reported and treated as potentially in flight even if the network write begins later. The project does not claim that a database commit can atomically stop a network write already authorized outside the transaction.

### Stale workers and ambiguous submissions

An expired lease fences database ownership but cannot cancel or prove the absence of a virtual-target request. Any expired `SUBMITTING` attempt becomes reconciliation-only. Automatic retry is permitted only after a durable transport result proves that the original request was not written, or after a target-specific absence/fencing mechanism also excludes a late original request. Generic absence of task, staged-image, or version evidence is inconclusive and moves the target to `NEEDS_ATTENTION`.

### Work leases and operation reservations

Short work leases govern one handler execution and may expire. Durable operation reservations govern physical blast radius from submission admission through update, reboot, version, and health resolution. Reconciliation transfers ownership of an operation reservation; lease expiry never releases it. An unresolved `NEEDS_ATTENTION` target retains its quarantine and safety reservation until a supported evidence path or an audited administrative resolution declares a safe release condition.

### Target ownership

A target-enrollment claim is created for every selected target in the same transaction that makes a campaign `READY`. It remains active until the parent campaign releases ownership. Target-update terminality alone does not release it. An unresolved or possibly active device operation also retains a target quarantine, including after a campaign reaches a terminal administrative state.

### Delivery identity

`command_id` identifies semantic work permanently. `delivery_generation` identifies a durable publisher intent series, not an SQS message ID or receipt handle. An uncertain or post-publish-crash `SendMessage` may create duplicate wakeups. Repair increments `delivery_generation` only when a new durable due-work generation is required. Workers reload, deduplicate, and authorize work from PostgreSQL state.

### State-machine requirements

Retry state retains an explicit `resume_step`; observation retries cannot return to a submission-capable path. `NEEDS_ATTENTION` has audited transitions for adopt-and-observe, terminal failure, and proof-backed retry. Campaign pause retains `suspended_phase` so canary execution, canary evaluation, and wave execution resume to the correct phase. Abort distinguishes work with no possible side effect from accepted or ambiguous operations that must continue observation.

### Failure budgets

The baseline promise is that new submission authorizations stop when an observed threshold is reached. The UI and evidence report the number of already authorized or physically active operations that remain exposed. The RFC does not claim that already admitted operations can be prevented from failing. A stricter campaign policy may reserve worst-case failure capacity for every active operation.

### Replay and restore

Decision replay retains normalized decision inputs, logical time, policy and reducer versions, relevant counters, result, and reason for the audit lifetime; hashes alone are insufficient. A restored database starts in recovery mode with submission disabled and fresh delivery generations. It reconciles against device or persistent simulator state that may be newer than the backup before an administrator can re-enable submission.

## Independent proof rules

The simulator records every received firmware request and every accepted operation independently of FleetForge state before applying any simulated response loss. A non-idempotent profile exposes duplicate requests as duplicates. `PROVEN_ABSENT` must independently agree with that journal and cannot relax the at-most-one-acceptance assertion.

Concurrency tests measure accepted but unresolved simulator operations, not only database counters. Pause tests include a barrier after admission and before request write. Amazon SQS Standard provider tests cover duplicate receipt, visibility-timeout redelivery, uncertain publish, and delivery-generation repair. Restore tests use an older database snapshot with a newer persistent device state. Property tests use an independent reference model, deterministic scheduler, shrinking, transition coverage, and retained failing schedules.

## Phase-order correction

The implementation order is:

1. Foundation and contracts.
2. Domain state machines and independent sequential model.
3. Persistence, generic due work, leases, enrollment claims, audit, and outbox.
4. Cloud development bootstrap for provider-dependent application services;
   the virtual-BMC backend remains locally runnable and cloud-independent.
5. Virtual-BMC contract spike that freezes selected DMTF fields, explicit
   simulator extensions, update/task/inventory behavior, limits, faults, and
   recovery evidence.
6. Deterministic persistent virtual-BMC backend and independent oracle.
7. Inventory/preflight and artifact integrity.
8. Campaign planning.
9. Generic orchestration, delivery generations, durable operation reservations, and deterministic concurrent model.
10. Firmware execution and reconciliation against the accepted synthetic contract.
11. API/auth, console, observability, and cloud hardening.
12. Load, fault, restore, deterministic replay, and portfolio release.

The existing numbered phase files remain stable for navigation, but their entry criteria and tasks must follow these dependencies. Phase 13 therefore has an early bootstrap slice required before provider-dependent gates and a later hardening/promotion slice.

## Claim boundary

The portfolio may claim only the accepted repository-owned virtual-BMC
contract and its measured scenarios. Vendor behavior, external-device
interoperability, real firmware installation, flash/power-loss resilience, and
management-network behavior remain explicitly unverified. Selected DMTF fields
and schema checks establish fixture discipline; they do not establish complete
Redfish conformance.
