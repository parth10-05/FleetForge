# V1-P05 composed evidence closeout plan

## Objective

Make the hosted `fault-harness-evidence` artifact prove the complete P05
composed-fault execution, rather than publishing a single acceptance/negative
control report after a package that executes multiple independent cases.

## Observed gap

The exact-SHA hosted workflow uploads a non-expired `fault-harness-evidence`
artifact, but the current test package has one `writeEvidence` call, from
`TestComposedHarnessStartsRealSixTargetSimulator`. Its generated report is
`P05-HARNESS-ACCEPTANCE-NEGATIVE-CONTROL`; the remaining composed cases have
no per-case retained report or durable index entry. This is insufficient for
the evidence contract in `docs/evidence/v1-p05/README.md`, which requires
case-level execution records and an index for all supported deterministic
cases.

## Design

Introduce a test-only, deterministic evidence recorder in `test/fault`:

1. Accept an explicit stable matrix case ID, scenario path, isolated schema,
   simulator status/journal checkpoint where present, asserted invariant, and
   outcome.
2. Write one redacted JSON report per executed composed case using an atomic
   temporary-file rename. Report names are derived only from stable case IDs;
   no network address, DSN, credential, callback body, raw journal record, or
   target identity is retained.
3. Write a package-level index only after every test that opted into the
   recorder has emitted its terminal report. The index lists stable case ID,
   report filename, scenario digest, outcome, and any intentionally deferred
   case without marking it passed.
4. Make report writes fail the owning test. Missing evidence must make the
   hosted fault job fail; an artifact upload alone can never convert absent
   case proof into a passing state.

## Case mapping

Map each existing composed test to its matrix case(s), without inventing
coverage:

| Existing composed behavior | Matrix case(s) |
| --- | --- |
| accepted restart plus duplicate/false-absence controls | P05-F01, P05-F12, P05-F13, P05-F14 |
| pre-write loss child retry | P05-F02, P05-F15 |
| post-accept loss adoption | P05-F03 |
| duplicate/late/lost callbacks | P05-F04, P05-F05, P05-F06 |
| rate-limited/unavailable observation | P05-F07, P05-F08 |
| task absence and inventory contradiction | P05-F09, P05-F10 |
| health contradiction | P05-F11 |
| administrator absence retry | P05-F16 |

P05-F17 through P05-F21 are durable PostgreSQL tests rather than composed
fault-harness cases. Their reports belong in a distinct PostgreSQL evidence
index only if the scope expands; they must not be falsely emitted by this
package.

## Affected surfaces

- `test/fault/composed_harness_integration_test.go`: replace the one-off
  evidence call with case-level, outcome-aware emissions at successful
  completion points.
- `test/fault/report.go` (new): report schema, redaction-safe validation,
  atomic writer, and package index writer.
- `test/fault/report_test.go` (new): deterministic filenames, no-secret
  fields, atomic output, duplicate case rejection, and index validation.
- `.github/workflows/foundation.yml`: add a post-test evidence completeness
  validator before artifact upload.
- `docs/evidence/v1-p05/README.md` and `fault-matrix.json`: document the
  artifact/index relationship and keep statuses honest until a new exact-SHA
  run produces the full report inventory.

## Error handling and invariants

- The writer rejects empty/unknown case IDs, non-SHA-256 scenario digests,
  duplicate reports, and `PASS` reports without an invariant string.
- Test code never emits a report from deferred or not-executed behavior.
- The index is deterministic and sorted by case ID.
- Failure paths leave a temporary file only; they never overwrite a valid
  report or report a case as passed.
- Hosted validation compares the index's case set to the explicit composed
  case manifest; adding a test requires consciously adding its mapping.

## Verification

1. Unit-test the recorder and validator.
2. Run each composed case locally against isolated PostgreSQL and inspect only
   report field names/values for redaction and deterministic coverage.
3. Run `go test -tags=postgresintegration ./test/fault/...` on the direct
   isolated endpoint.
4. Push, then inspect the exact-SHA `fault-harness-evidence` artifact:
   every mapped P05-F01 through P05-F16 appears exactly once, both negative
   controls are explicit, the restore case is explicit, and no secret-bearing
   fields are present.
5. Keep package status `NEEDS_ATTENTION` until that exact hosted artifact and
   the final CI run have both been inspected.
