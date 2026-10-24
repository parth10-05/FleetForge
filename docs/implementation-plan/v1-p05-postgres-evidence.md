# V1-P05 durable PostgreSQL evidence closeout plan

## Objective

Close the remaining evidence gap for `P05-F17` through `P05-F21` without
misrepresenting them as simulator-composed cases. The existing fault-harness
artifact and index cover `P05-F01` through `P05-F16`; these five matrix cases
are separate durable PostgreSQL invariants and require their own retained
reports in the same hosted artifact.

## Current evidence boundary

`docs/evidence/v1-p05/fault-matrix.json` defines 21 required IDs. The
composed-harness recorder deliberately validates only `P05-F01` through
`P05-F16`; extending that manifest would falsely claim that database-only
tests crossed the simulator boundary. The PostgreSQL integration workflow
already runs every source test named by the remaining matrix rows, but retains
no case-level report or index.

## Explicit mappings

| Matrix case | Source test | Invariant retained in report |
| --- | --- | --- |
| `P05-F17` | `TestDueWorkClaimFencesStaleCompletion` | A stale worker cannot complete work after the durable lease fence changes. |
| `P05-F18` | `TestAdmissionStaleEpochRollsBackCapacityAndEvidence` | Epoch conflict rolls back reservation and evidence atomically. |
| `P05-F19` | `TestCommittedPauseAfterAdmissionRetainsResidualExposure` | A committed pause after admission retains, rather than erases, residual exposure. |
| `P05-F20` | `TestAdminTerminalDispositionPreservesOrReleasesThreeScopeCapacity` | Terminal disposition changes only the permitted scope reservations. |
| `P05-F21` | `TestFencedAmbiguousSubmitSchedulesReconciliationWithoutSecondSubmit` | Submit uncertainty schedules reconciliation and cannot create a second submission. |

## Design

1. Add a PostgreSQL-test-only recorder in `test/integration/postgres` gated by
   `FLEETFORGE_POSTGRES_EVIDENCE_DIR`. It accepts an explicit P05 case ID and
   static invariant, then defers writing until its owner test succeeds.
2. Each redacted report contains only a schema version, stable case ID,
   source-test identifier, invariant, and `PASS`. It contains no DSN, schema
   name, UUID, target identity, SQL, credential, or raw database data.
3. A package `TestMain` writes `postgres-index.json` only when all five expected
   IDs have exactly one valid report. Missing, duplicate, or unexpected reports
   fail the integration package before artifact upload.
4. Configure the existing PostgreSQL CI step to set a sibling evidence
   directory under `$RUNNER_TEMP/fleetforge-fault-evidence`. The existing
   `fault-harness-evidence` upload then contains both `index.json` (composed)
   and `postgres-index.json` (durable PostgreSQL), with disjoint filenames.
5. Keep the fault matrix status `NEEDS_ATTENTION` until a new exact-final-SHA
   workflow succeeds and its combined artifact is inspected. Only then promote
   individual case statuses and the top-level closeout state.

## Error handling and correctness rules

- Registration fails for non-matrix IDs or empty invariants.
- Test cleanup writes nothing after an assertion failure.
- Writes use a `.tmp` file and rename, reject duplicate final names, and use
  owner-only permissions.
- The index requires the exact five-case set; passing a focused test with the
  evidence variable set intentionally fails completeness validation.
- The composed and PostgreSQL recorders do not share case manifests, preventing
  a boundary regression from being hidden by evidence bookkeeping.

## Verification

1. Compile both tagged test packages.
2. Run the five mapped PostgreSQL source tests against the isolated direct
   Neon endpoint with the evidence directory set; inspect only report filenames
   and schema-safe fields.
3. Run the complete `./test/integration/postgres/...` package with the same
   directory; verify `postgres-index.json` has exactly `P05-F17`–`P05-F21`.
4. Run the complete fault package and PostgreSQL package together as the CI
   workflow does, then confirm both indexes coexist.
5. Push and inspect the exact-SHA hosted artifact and workflow before changing
   closeout status.
