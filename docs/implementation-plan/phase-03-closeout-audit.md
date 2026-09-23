# Phase 03 closeout audit

**Verified code SHA:** `037d637` (`fix(persistence): canonicalize schema compatibility fingerprint`)

**Gate status:** passed — [Foundation verification #25](https://github.com/parth10-05/FleetForge/actions/runs/35492849379), 2026-09-20, 1m29s.
This document is not a claim of a provider deployment, queue delivery,
firmware execution, 1,000-target throughput, or virtual-target contract proof.

## Scope and migration inventory

| Version | Purpose |
|---:|---|
| 00001 | Core targets, artifacts, campaigns, waves, frozen campaign targets, and target updates |
| 00002 | Enrollment, admission, and durable operation-reservation safety claims |
| 00003 | Attempts, observations, idempotency, decisions, append-only audit, outbox, and due work |
| 00004 | Global/site/rack capacity counters |
| 00005 | Fenced outbox claims |
| 00006 | Migration, application, and diagnostic role boundaries |

The first Phase 03 persistence release is the honest v1 baseline. P3-16 tests
both empty-to-latest and the last supported in-phase snapshot (version 5) to
version 6. Rollback policy is forward repair plus backup/restore rehearsal;
all migration Down sections are intentionally non-destructive. There is no
previous application binary in this release, so old-app/new-schema execution
is a required later-release gate rather than fabricated evidence.

## Task-to-code-to-proof reconciliation

| Tasks | Primary implementation | Executable proof |
|---|---|---|
| T01, T03-T05 | config.go, connection.go, ADR-009 | config/connection tests; CI build and PostgreSQL gate |
| T02, T12 | 00006_roles_and_privileges.sql | roles_readiness_integration_test.go privilege-negative test |
| T06 | ReadinessProbe | same test distinguishes unavailable from incompatible schema |
| T07-T09, T14 | 00001_core_entities.sql | core_schema_integration_test.go |
| T10-T12A, T36A | 00002_ownership_and_operation_safety.sql, enrollment.go | ownership/enrollment/admission integration tests |
| T13 | migration index comments and db/queries/README.md | query-map review plus PostgreSQL suite |
| T15-T20 | db/queries, sqlc package, mapper and unit-of-work code | mapper and unit_of_work_integration_test.go |
| T21-T25 | idempotency.go | idempotency_integration_test.go |
| T26-T27, T36C | audit_replay.go, 00003_durable_records.sql | audit replay and durable-record integration tests |
| T28-T31 | outbox.go, 00005_outbox_claim_fencing.sql | outbox_integration_test.go |
| T32-T36 | due_work.go, Q10/Q21-Q23 | due_work_integration_test.go |
| T36B | enrollment/admission/reservation schema and transaction paths | ownership, admission, due-work integration tests |
| T37-T40 | retention.go, integrity.go, retention operations runbook | retention/unit tests and integrity integration proof |

The planned 1,000-row EXPLAIN (ANALYZE, BUFFERS) capture is documented in the
connection/query-plan method but remains a cloud/bootstrap performance gate; no
measurement is represented as complete here.

## Integration test identity map

| IDs | Current evidence |
|---|---|
| IT-01-03 | unit_of_work_integration_test.go: revision conflict and atomic rollback |
| IT-04-05 | idempotency_integration_test.go: identical request convergence and hash conflict |
| IT-06-08 | due_work_integration_test.go: claim uniqueness, stale fencing, expired repair |
| IT-09 | ownership_schema_integration_test.go and enrollment_integration_test.go |
| IT-10 | outbox_integration_test.go: claim/publish fencing and quarantine |
| IT-11 | mapper corruption tests fail closed |
| IT-12-13 | migration_compatibility_integration_test.go: v5 upgrade fingerprint and empty latest fingerprint |
| IT-14 | retention and integrity tests preserve active/referenced records |
| IT-15-16 | ownership/admission/due-work tests preserve enrollment and reservations |
| IT-17 | transactional_history_integration_test.go plus migration-lock and ownership races |
| IT-18 | audit replay tests reconstruct retained decision inputs after observation removal |

## Concurrency and error audit

The deterministic order is campaign, target update, enrollment, global/site/rack
capacity, admission, reservation, due work/outbox, then audit/decision append
(ADR-021). The P3-17 schedule starts two workers concurrently against one
frozen target. The committed worker records its generated request IDs, revision
0 to 1, three reservations, and full history; the other must return a
classified revision, admission, or duplicate conflict. Independent existing
schedules cover enrollment racing, two due-work claimers, stale lease tokens,
expired repair, and concurrent migrators.

| Class | Boundary behavior |
|---|---|
| Expected revision or stale epoch | ErrRevisionConflict / ErrAdmissionBlocked; no partial history |
| Duplicate admission/idempotency identity | ErrAdmissionDuplicate / deterministic idempotency conflict |
| Lease/publisher token stale | ErrLeaseConflict / ErrOutboxClaimConflict |
| Serialization or deadlock | SQLSTATE 40001/40P01; safe outer caller owns a bounded retry budget |
| Unreachable database | ErrDatabaseUnavailable |
| Wrong migration version/corrupt state | ErrDatabaseSchemaIncompatible / corruption error, fail closed |

## Skeptical transaction-boundary review

ApplyAggregateMutation and AdmitOperation use short serializable transactions
for state, audit, decision input, outbox, due-work, admissions, and capacity.
ClaimOutbox commits before EventPublisher.Publish is called; the only publisher
interface use is therefore outside the claim transaction. A source scan of
internal/persistence/postgres finds no HTTP, Redfish, SQS, or storage client.
This is source-level proof only; later Phase 08/09 needs an instrumented
blocking-adapter integration test once a real worker/device path exists.

## Privilege, restore, and provider limits

Migration 00006 makes audit update/delete and Goose-history mutation fail for
the application role; the diagnostic role cannot insert operational records.
The generated schema fingerprint compares normalized tables, columns, indexes,
constraints, and Goose history, not a vendor-specific physical plan.

Neon pooled/direct endpoint smoke, actual provider limits, backup export and
isolated restore rehearsal, SQS redelivery, Redfish-shaped side effects, virtual-target
compatibility, and 1,000-target performance remain pending their named future
phases. The PostgreSQL CI service is correctness evidence, not Neon evidence.

## Final CI evidence

Foundation verification #25 at `037d637` passed the generated SQL check,
formatting, vet, unit/property suite, real PostgreSQL integration suite, Linux
race, and builds. A later documentation-only commit does not alter the verified
code; any later code change requires a replacement full hosted run.
