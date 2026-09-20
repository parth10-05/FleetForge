# Phase 03 P3-15 to P3-18 Implementation Plan

## Purpose and current baseline

This plan completes the remaining Phase 03 cards on
`codex/phase-03-persistence-outbox` after P3-14 and the PostgreSQL cursor
repair at `d38bfe6`. The authoritative acceptance environment is the existing
GitHub Actions ephemeral PostgreSQL service and its hosted Linux race gate.
Local Windows checks remain database-free and do not constitute PostgreSQL or
race evidence.

No card in this run adds a provider integration, queue client, HTTP business
route, Redfish call, cloud deployment, or firmware-side effect. Neon pooled
and direct endpoint smoke testing remains an early cloud-bootstrap proof, not
Phase 03 evidence.

## Invariants carried into the run

1. Every multi-row writer retains ADR-021 lock order: campaign, target update,
   enrollment, capacity, admission, reservation, work/outbox, then audit and
   decision input.
2. A database transaction never invokes a network, storage, queue, or exporter
   dependency. Publisher execution stays after an outbox claim commits.
3. Work-lease expiry cannot release an admission, reservation, capacity count,
   or target enrollment.
4. Persisted values are decoded through strict domain parsers; malformed or
   unknown values are corruption, not a default state.
5. Applied migrations are immutable. The migration path only expands the
   schema; no destructive down-migration is used as a release rollback.

## P3-15: roles, readiness, budgets, and query plans

### Files and interfaces

- Add a sequential migration that creates the dedicated migration,
  application, and diagnostic roles where PostgreSQL permits it, assigns the
  role grants, and records no credentials.
- Add `internal/persistence/postgres/readiness.go` with a narrow readiness
  probe result that distinguishes unavailable connectivity, missing/incomplete
  migration history, and a schema version outside the supported range. It must
  redact connection details and perform bounded context-based work only.
- Extend connection configuration only with explicit pool and timeout values;
  retain the application-pool versus direct-migration separation.
- Add PostgreSQL-tagged integration tests that use `SET ROLE`/role-specific
  connections to prove application audit update/delete and schema-history
  mutation fail, while diagnostic mutations fail. Tests must gracefully report
  an unsupported role-provisioning fixture rather than weakening the schema.
- Add a checked-in performance evidence document containing reproducible seed
  shape, exact `EXPLAIN (ANALYZE, BUFFERS)` commands, index/query mapping, and
  a clearly labelled representative result captured in CI-compatible
  PostgreSQL. It must not claim production or Neon capacity.
- Add a connection-budget worksheet with named ECS API/worker max-instance,
  per-role pool, migration, diagnostic, and operational-headroom variables;
  show the arithmetic and the resulting maximum database connections.

### Data and error flow

The readiness probe first obtains a bounded pool connection and maps failures
through existing SQLSTATE/connectivity classification. Only after a successful
connection does it query `goose_db_version` and compare the current maximum
applied version to the compiled supported range. This prevents an unreachable
database from being misreported as a schema incompatibility. It performs no
migrations and no retry loop under a held transaction.

Role tests create only ephemeral test-scoped roles/schema resources through
the CI superuser service account. Normal code does not assume it owns roles.
The migration role is the only owner of history/schema DDL; the application
role can insert/select audit rows but cannot alter history or audit rows; the
diagnostic role has read-only data access.

### Proof

- Unit tests cover readiness classification and DSN redaction.
- Tagged tests cover role-negative operations and compatible versus
  incompatible schema states.
- CI captures/validates the named query plans and lock/transaction measurement
  command output as an artifact or checked-in synthetic evidence.

## P3-16: migration compatibility and restore inputs

### Files and interfaces

- Extend `test/integration/postgres` with an explicit empty-to-latest migration
  test and a last-supported-snapshot-to-latest test. Snapshot setup must use
  the immutable migration prefix rather than copying a mutable live schema.
- Add a deterministic schema fingerprint helper that queries normalized
  catalog metadata (tables, columns, constraints, indexes, and migration
  versions), serializes it in stable order, and hashes the serialization.
- Add a compatibility/restore runbook documenting baseline v1, the supported
  migration window, backup/export input requirements, restore rehearsal
  commands for a future release environment, and the non-applicable old-binary
  gate for this first persistence release.

### Concurrency and failure handling

Two concurrent `cmd/migrate`-equivalent invocations target the same generated
schema and must converge under the existing advisory lock: one applies the
pending history and the other observes the same final version. A failure must
leave a valid Goose history and must not use down migrations to recover.

The tests prove migration compatibility only for the actual Phase 03 prefix.
They do not fabricate a pre-Phase-03 production schema or claim an old
application binary has been executed against the expanded schema.

### Proof

- Empty-to-latest fingerprint equals the expected current fingerprint.
- Prefix-snapshot-to-latest fingerprint equals the same expected fingerprint.
- Concurrent migrators return successfully and record a single ordered history.
- Runbook explicitly labels production restore and old-binary validation as
  later release gates.

## P3-17: transactional-history concurrency gate

### Files and interfaces

- Add `history_*_integration_test.go` files with deterministic seeds,
  barrier-controlled schedules, worker IDs, lease tokens, revisions, and
  maximum-active-reservation observations.
- Add a test-only independent oracle that reads final durable state and checks
  ownership uniqueness, admission uniqueness, audit completeness, reservation
  retention, stale-result rejection, and capacity accounting without calling
  the production transaction helpers it is judging.
- Reuse established admission/enrollment/due-work APIs rather than introduce a
  generic repository or a worker loop.

### Schedules

1. Completion, repair, and stale result race on one due-work/reservation
   lineage: only the current lease may commit; repair cannot release capacity.
2. Two campaigns race readiness/admission on the hot rack while an unrelated
   rack is admitted: exactly one target enrollment/admission exists; the
   unrelated rack makes progress.
3. Campaign epoch/pause-like mutation overlaps an admission attempt: outcomes
   are either safely admitted before the fencing commit or rejected after it;
   there is no duplicate admission or audit gap.
4. Repeated bounded schedules deliberately induce serialization/deadlock-prone
   overlap. SQLSTATE retries are bounded at a safe outer boundary and every
   terminal observed result preserves all invariants.

### Proof

The integration suite runs each schedule repeatedly against CI PostgreSQL and
emits the seed/schedule metadata on failure. The final GitHub Actions run also
supplies the required hosted Linux `go test -race ./...` proof; local Windows
does not claim race success.

## P3-18: closeout and independent audit

### Deliverables

- Create the Phase 03 closeout audit with a task-to-code-to-test-to-CI matrix
  for P03-T01 through T40 and P03-IT-01 through IT-18.
- Include the exact final commit and GitHub run URL, migration inventory and
  fingerprint, index rationale, connection-budget math, query-plan method,
  concurrent schedules, SQLSTATE/error-class table, privilege proof, replay
  proof, retention proof, and known limitations.
- Add a separate skeptical transaction-boundary review that enumerates each
  transaction entry point, its locks/writes, and why it cannot perform network
  I/O while a lock is held.

### Acceptance rule

Closeout is committed only after the final pushed SHA has a passing Foundation
verification run containing normal Go tests, PostgreSQL integration tests,
generation/format/vet checks, builds, and hosted Linux race. Any verified
failure is fixed in a focused follow-up commit before the closeout is marked
complete. The artifact must state that Neon, queue-provider, Redfish, and
production restore proofs remain outside Phase 03.

## Delivery and validation order

1. Implement P3-15, run local unit/format/vet/build checks, commit, push, and
   inspect the PostgreSQL CI gate.
2. Implement P3-16 using the P3-15 baseline, then repeat the focused and CI
   verification.
3. Implement P3-17 only after P3-16 migration behavior is proven. Run the
   history package repeatedly in CI; do not substitute mocks for database
   histories.
4. Create P3-18 evidence after the final code change. Push the closeout and
   inspect the clean hosted run at that final SHA.

At every commit, leave `.agents/`, `AGENTS.md`,
`docs/implementation-plan/phase-03-antigravity-handover.md`, and
`skills-lock.json` untracked and untouched.
