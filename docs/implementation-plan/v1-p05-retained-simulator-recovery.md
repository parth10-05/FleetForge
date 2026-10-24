# V1-P05 retained simulator recovery remediation plan

## Objective

Make the real-process composed fault harness prove that a simulator which has
accepted an update can persist a coherent engine snapshot, stop, recover the
same retained run, and serve the prior journal and target truth without
returning the misleading `Synthetic retained state is unavailable` response.

## Observed boundary

`TestComposedHarnessStartsRealSixTargetSimulator` accepts one update through
the public Redfish endpoint, then reads the independent retained journal and
restarts the child process against the same `FLEETFORGE_SIMULATOR_RUN_ROOT`.
The request path appends its independent acceptance evidence before invoking
`Handler.checkpoint`; any failure of the checkpoint callback is surfaced as
HTTP 503. The callback is `checkpointWriter.Write`, which obtains one core
snapshot and asks `simulatorstate.Run.WriteStateSnapshot` to bind it to the
current journal tail atomically.

The repair must preserve the existing fail-closed rule: an update is never
acknowledged when its recovery state could not be made durable. It must not
turn a checkpoint failure into a success response or fabricate journal state.

## Investigation sequence

1. Reproduce the tagged composed test and retain its child-process output and
   the actual checkpoint error. Confirm whether the failure originates in core
   snapshot capture, journal read/append ordering, atomic snapshot replacement,
   or process ownership on Windows.
2. Inspect the retained run after the failure: manifest, snapshot validity,
   journal chain, ownership-lock release, and any invalid marker. Never modify
   the run while diagnosing it.
3. Identify the smallest durable-state boundary that can enforce a coherent
   `(engine projection, journal checkpoint)` pair without concurrent writers
   racing the snapshot file.

## Intended implementation

If reproduction confirms a writer coordination issue, introduce a single
retained-state checkpoint primitive that serializes the engine snapshot with
the journal tail it certifies. The primitive will:

1. Keep the existing mutex across snapshot acquisition and filesystem write.
2. Read only a fully flushed journal tail and bind its sequence/hash to the
   snapshot.
3. Use the existing atomic snapshot-write protocol; no direct overwrites.
4. Return the original durable error to the request handler, preserving its
   503 fail-closed behavior.
5. Avoid holding any database lock or issuing target/network I/O.

If the reproduction instead identifies child-process shutdown or Windows
ownership timing, make the process lifecycle wait for the simulator's orderly
exit and released owner lock before restart. Do not weaken owner exclusivity.

## Affected surfaces

- `cmd/redfish-simulator/main.go`: checkpoint orchestration and graceful
  process shutdown/recovery handoff, only if the failure is here.
- `internal/platform/simulatorstate/run.go`: retained snapshot/journal binding,
  only if the atomic persistence layer is the source.
- `internal/delivery/redfishsim/router.go`: unchanged response contract unless
  a narrow callback sequencing hook is required.
- `test/fault/composed_harness_integration_test.go`: real-process regression
  proof for acceptance, checkpoint, restart, journal continuity, and recovered
  firmware version.
- Focused unit tests near any changed writer/state helper for invalid or
  partially retained state.

## Acceptance evidence

The repair is complete only when:

1. the composed tagged test passes with its actual child process;
2. the retained journal still has ordered request-received then update-accepted
   evidence for the original attempt;
3. after restart, run ID, journal entry count/tail hash, and firmware version
   equal the pre-stop checkpoint;
4. negative controls still reject duplicate acceptance and a false
   proven-absent claim;
5. focused simulator packages pass locally and exact-SHA hosted CI passes.

`P05-F14` remains `IMPLEMENTED_IN_HARNESS` rather than closed until hosted
evidence covers the final candidate SHA.
