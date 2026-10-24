# V1-P05 composed fault and acceptance harness

## Purpose

Close the P05 proof gap with an executable harness that runs a real
`redfish-simulator` process against a real PostgreSQL schema, drives the
durable worker roles through their public process boundaries, and retains
redacted evidence. This is not a replacement for the independent journal: the
simulator journal remains the source of truth for received and accepted target
operations.

## Non-goals

- It does not claim external-device, cloud-provider, or Cognito/JWKS proof.
- It does not replace the existing focused Postgres integration tests.
- It does not persist connection strings, credentials, authorization headers,
  artifact bytes, or callback bodies in source or retained evidence.

## Architecture

The runner is a tagged integration test in `test/fault` that receives the
existing CI PostgreSQL DSN. For each case it creates a fresh random schema,
runs migrations in that schema, seeds a campaign whose six targets span at
least two racks, and starts the simulator using a copied scenario file and an
isolated run root.

The runner starts and stops the simulator and selected worker commands as
child processes. It drives only their published HTTP, environment, durable-work
and retained-run interfaces. It must never import simulator core state into
the checker or mutate internal simulator state directly.

After every schedule the checker reads:

1. the simulator’s retained hash-chained journal and final checkpoint;
2. the PostgreSQL attempt, admission, due-work, reservation, and receipt rows;
3. process exit/termination observations; and
4. a redacted case transcript with seed, scenario digest, logical bounds, and
   assertion outcomes.

The application oracle validates that each accepted durable attempt is backed
by retained independent journal evidence. The checker additionally asserts at
most one accepted simulator operation per intended target update.

## Files and interfaces

| Area | Change |
| --- | --- |
| `test/fault/composed_harness_integration_test.go` | Tagged test harness, isolated schema lifecycle, child-process lifecycle, readiness polling, journal/DB collection, negative controls, and case execution. |
| `test/fault/testdata/v1-p05/*.json` | Immutable scenario copies or generated manifests for the 21 supported schedules. The existing six-target fixtures are reused where applicable. |
| `test/fault/report.go` | Redacted JSON report types and deterministic serialization; no DSN or secret fields. |
| `docs/evidence/v1-p05/` | Committed format/specification and only redacted outputs produced by an executed run; never a hand-written success claim. |
| `.github/workflows/foundation.yml` | Runs the tagged harness against the workflow PostgreSQL service, uploads the report/checkpoint/journal/DB-summary artifacts, and fails when required case or negative-control evidence is absent. |

## Case matrix

The initial matrix must enumerate exactly the supported V1 synthetic behavior
and label deferred behaviors explicitly. It includes healthy acceptance,
post-accept disconnect, lost/duplicate/late event, task disappearance,
pre-write failure, pause before and after admission, stale worker, duplicate
delivery, reservation/epoch conflict, 429/503 observation pressure, version
and health contradiction, retry budget exhaustion, admin proof-backed absence,
restore against newer simulator state, and the two negative controls. The
matrix source carries a stable case ID, scenario fixture, seed, expected
terminal/attention result, and the invariant checked.

The negative controls deliberately inject a second accepted request and a
false `PROVEN_ABSENT` durable resolution. The harness must observe its oracle
or invariant assertion fail; a control that passes is itself a harness failure.

## Execution sequence

1. Allocate paths, free loopback ports, schema, and redacted case ID.
2. Copy/decode the scenario and record its SHA-256; reject fewer than six
   targets or fewer than two racks before starting a process.
3. Launch the simulator, wait for health, and seed the matching durable
   campaign/artifact/target records in the scoped schema.
4. Start only the worker roles necessary for the scheduled transition; record
   every deliberate kill/restart or database-unavailable interval.
5. Poll durable state with bounded deadlines. Callback delivery can accelerate
   `TASK_OBSERVE` only; polling/reconciliation remains authoritative.
6. Stop children gracefully where possible, capture final journal checkpoint,
   export a selected redacted DB summary, and execute the independent oracle.
7. Serialize the report atomically. A passing run requires every positive case
   to satisfy its invariant and both negative controls to fail as intended.

## Restore schedule

The restore case records a checkpoint after a post-accept state, advances the
simulator to a newer truth, stops it, then restarts against the retained run
root. The checker verifies journal continuity, recovered task state, and that
FleetForge observes/adopts rather than sends a new submission.

## Safety and error handling

Each child receives a context deadline and process-group cleanup. The harness
is responsible for deleting only its generated temporary paths and scoped
database schema. It retains no secret-bearing environment or raw request data.
Unexpected process output is attached only after line/size redaction. A failed
case keeps its redacted transcript and returns an error; it never auto-retries
a target submission.

## Verification and closeout evidence

Local Windows runs may execute Docker-free structure/unit checks and an
explicit Neon-backed schema test, but the hosted Linux workflow is authoritative
for the tagged Postgres/race path. The closeout package must cite the exact
final commit SHA, workflow URL/run ID, artifact names and digests, all case
seeds, matrix status, negative-control outcomes, restore result, and retained
limitations. It must state `NEEDS_ATTENTION` until that evidence exists.
