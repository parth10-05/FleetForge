# FleetForge Phase 02 Verification Evidence

> **Audit status (2026-09-13): REMEDIATION PENDING HOSTED CI.**
> [The Phase 02 audit record](phase-02-audit-2026-09-13.md) found a retry
> resume-step bypass, untyped safety inputs, misplaced reference models,
> incomplete properties, and no hosted Linux race evidence. The local
> remediation sequence ends at `5afa1af`; hosted Linux verification remains
> required before this becomes a Phase 03 readiness declaration.

**Scope:** pure domain model and state machines only. No database, queue,
network client, background timer, Docker workload, or device side effect was
added.

**Branch:** `codex/phase-02-domain-model`  
**Remediation head before hosted verification:** `5afa1af`
**Date:** 2026-09-13

## Delivered contract slices

| Area | Evidence commits |
|---|---|
| Domain boundaries, values, evidence, states, and transition catalogs | `5a95949` through `22ccbe8` |
| Campaign and target-update reducers | `ad5aafa`, `fc2297e` |
| Waves, policies, canary contract, and administration state | `96ac0dd`, `2eeda89`, `55096e1`, `58e5e03` |
| Versioned events and deterministic replay | `70cf963`, `df6d03c` |
| Retry-resume safety fence | `6f2fa0c` |
| Typed policy, admission, absence, and success proof inputs | `bc82550` |
| Test-only independent reference models | `4733716` |
| Directed P02-PT-01 through P02-PT-11 coverage | `5afa1af` |
| Fuzz hardening and invariant correction | `34c72b5`, `1816e25` |

The randomized campaign suite found an invalid state retention path while
aborting from `PAUSED`: the captured suspended phase remained on `ABORTING`.
Commit `1816e25` clears it, adds reusable campaign/target invariant validators,
and locks the behavior with a regression test.

## Local verification performed

| Gate | Result |
|---|---|
| `go test -count=1 ./...` | Passed after the final invariant correction. |
| `go vet ./...` | Passed. |
| `git diff --check` | Passed before the evidence commit. |
| Directed property suite | Passed with `go test -count=1 ./test/domainmodel/... -args --rapid.checks=200`. Named subtests map P02-PT-01 through P02-PT-11 and include directed coverage for campaign running/evaluation states, all retry resume steps, attention exits, pause/abort epochs, duplicate commands, and stale evidence. |
| Independent-model mutation proof | Passed: the handwritten model rejects a mismatched retry exit and a deliberately incorrect campaign completion outcome. |
| Replay fixture/determinism suite | Passed. |
| Short actual fuzzing | Value parser, event decoder/canonicalizer, campaign reducer, and target-update reducer passes completed with `-fuzztime=2s`. |

The local `go test -race -count=1 ./internal/domain/...` gate could not build
on this Windows host because its C compiler reports:

```text
# runtime/cgo
cc1.exe: sorry, unimplemented: 64-bit mode not compiled in
```

This is a host toolchain limitation, not a failing domain test. The workflow
now runs the directed 200-check suite and `go test -race ./...` on Ubuntu;
the immutable run URL and final SHA are pending the pull-request run.

## Deliberate Phase boundaries

- The deterministic canary selector consumes an already-frozen candidate set;
  candidate discovery and final stratification remain later-phase work.
- Policy decisions are pure inputs/outputs; queues, sleeps, remote probes,
  persistence, operation reservations, and outbox publication remain deferred.
- Events are bounded domain contracts only. Transport envelopes and durable
  publication are Phase 03 work.
