# FleetForge Phase 02 — Detailed Implementation Execution Plan

## Status and approval boundary

**Status:** Planned on `codex/phase-02-domain-model`; implementation is pending a
separate project-owner approval.

This document turns the normative Phase 02 contract into a small-commit
execution sequence for the current Phase 01 baseline. It does not amend the
approved architecture or expand Phase 02 into persistence, transport, cloud,
or device work.

The Phase 02 deliverable is a deterministic Go correctness kernel. Its inputs
are complete domain facts; its outputs are decisions, immutable aggregate
snapshots, stable reason codes, event descriptions, and next-action intents.
It must not itself perform I/O or authorize an unrecorded device action.

## Verified baseline

| Item | Verified state | Implementation consequence |
|---|---|---|
| Starting commit | `b9ccc1b` on `main` before this branch was created | Keep Phase 01 repairs out of all Phase 02 behavior commits. |
| Feature branch | `codex/phase-02-domain-model` | Commit only coherent Phase 02 slices here; no history rewrite or squash. |
| Existing domain code | `internal/domain/doc.go` is a Phase 01 placeholder | New packages begin from a deliberately empty domain boundary. |
| Architecture test | Existing check blocks only direct `net/http`, `database/sql`, and OpenTelemetry imports | P02-00 must broaden it to the declared dependency graph and prevent indirect infrastructure imports. |
| Aggregate commands | `make verify` runs Go lint/test/race/build and web typecheck/lint/test/build | Add narrow domain targets without weakening this aggregate command. |
| Race evidence | Host race execution is blocked by its 64-bit C toolchain; a prior Linux GitHub Actions run passed the Go race suite | Record this limitation; use Linux CI evidence for every relevant Phase 02 race gate. |
| Docker | Explicitly deferred by the project owner | Do not build, test, alter, or add container workflows in Phase 02. |

## Scope and non-goals

### In scope

- Canonical domain values, strict state parsers, bounded evidence, and stable
  reason-code contracts.
- Campaign, target-update, wave, artifact, and inventory lifecycle contracts.
- Pure reducers and policy functions with explicit time, revision, epoch,
  policy-version, evidence, and ordering inputs.
- Canonical event and replay contracts.
- Independent sequential model, generated properties, fuzzing, contract
  fixtures, architecture tests, and phase evidence.

### Explicitly out of scope

- PostgreSQL, SQL, migrations, transaction semantics, repositories, leases,
  target-enrollment claims, durable operation reservations, audit persistence,
  or outbox publishing.
- HTTP/OpenAPI behavior, authentication, UI state, worker loops, queues,
  goroutines, clocks, sleeps, logging, metrics, traces, filesystem access, or
  configuration/environment reads.
- Redfish parsing, firmware bytes, device networking, object storage, cloud
  SDKs, provider resources, and Docker work.

## Design decisions fixed for this phase

### Dependency direction

Production packages may import only the Go standard library and the inward
domain dependencies below. Test-only property tooling is allowed in `_test.go`
or `test/domainmodel` only.

```text
value, reason                    -> standard library
evidence, artifact, inventory,
policy                            -> value
campaign                          -> value, reason, policy
wave                              -> value, reason
targetupdate                      -> value, reason, evidence, policy
event                             -> value, reason
replay                            -> campaign, targetupdate, event
test/domainmodel                  -> public domain contracts and test library
```

The P02-00 architecture check must reject imports of repository layers outside
`internal/domain`, any cloud/SQL/HTTP/logging/telemetry/queue/UUID-generation
dependency, and any import edge absent from this graph. It must parse every Go
production file under `internal/domain`, not merely the root package.

### Determinism, ownership, and immutability

- No package-level mutable state, default random seed, wall-clock call, or
  filesystem/environment read may enter `internal/domain`.
- Reducers accept a value snapshot and return a distinct result. Rejections and
  no-ops leave the supplied aggregate unchanged and cannot expose retained
  mutable slices or maps.
- `revision` advances exactly once for a successful state change. Rejected,
  stale, duplicate, and already-satisfied operations do not advance it.
- Commands carry an expected revision. A stale revision fails before any
  side-effect-admission intent can appear in the decision.
- Campaign pause and abort increment `command_epoch` once if and only if they
  change the campaign. An admission from an earlier epoch remains represented
  as potentially in flight; neither command claims to cancel an already
  granted external authorization.
- Duplicate `command_id` behavior is deterministic only when the caller
  supplies the already-recorded command result. Phase 03 owns durable command
  identity persistence.

### Safety policy decisions

- Failure-budget comparisons use integer/rational arithmetic. The frozen Phase
  02 rule is **inclusive**: stop new submission admission when
  `failed / eligible >= numerator / denominator`. Skipped and cancelled targets
  are excluded from the eligible denominator; a zero denominator yields an
  unsafe/no-release decision rather than a divide-by-zero default.
- A retry stores one explicit `resume_step`. Observation failures may resume
  only at `WAITING_REBOOT` or `VERIFYING`; they never recreate a submission
  path.
- `PROVEN_ABSENT` is a caller-provided normalized, independent proof fact. No
  absence of task/version evidence is sufficient. `INCONCLUSIVE` retains or
  enters `NEEDS_ATTENTION` and cannot authorize retry.
- Success requires expected firmware version, every required health check,
  fresh evidence under the frozen policy, and no contradictory terminal
  evidence. Task completion alone is never success.
- The next wave can release only when the immediately preceding ordinal is
  terminal and has a successful immutable evaluation under the frozen policy.

## Package and file map

```text
internal/domain/
  doc.go
  value/{ids,temporal,identity,firmware,policy_values}.go
  reason/code.go
  evidence/{evidence,classification}.go
  campaign/{state,command,decision,transition_catalog,reducer}.go
  targetupdate/{state,step,command,decision,transition_catalog,reducer}.go
  wave/wave.go
  artifact/artifact.go
  inventory/target.go
  policy/{failure_budget,retry,maintenance_window,verification,canary}.go
  event/{event,payloads}.go
  replay/replay.go
test/domainmodel/{campaign_model,target_model,generators}.go
test/fixtures/domain/v1/{campaign-transitions,target-update-transitions,
  event-contracts,replay-campaign.golden,replay-target-update.golden}.json
```

Each production package owns a small cohesive behavior. Package filenames may
be combined when that improves cohesion, but neither dependency direction nor
the listed responsibility may change without updating this document and the
normative Phase 02 plan.

## Decision result contract

All reducers use an equivalent immutable decision shape. Exact Go type names
may vary by aggregate, but every complete result contains:

| Field | Contract |
|---|---|
| Classification | `allowed`, `conflict`, `invalid_input`, `unsafe_condition`, or `no_op`; a rejected result has exactly one rejected class. |
| Aggregate | Resulting distinct aggregate value; unchanged for rejected/no-op paths. |
| Revision and epoch | Resulting values, with explicit one-step mutation semantics. |
| Reason code | Registered, bounded, machine-stable value; no human prose or raw device data. |
| Events | Deterministically ordered, bounded descriptions; zero for a no-op/rejection unless a future contract explicitly says otherwise. |
| Counter delta | Explicit attempt/budget/aggregate-counter effects, never inferred from a side effect. |
| Next intent | Pure later-layer instruction such as validate, evaluate, admit submission, observe, reconcile, or no further action. It is not a queue message or HTTP response. |
| Safety retention | Explicit suspended phase, retry resume step, and/or unresolved-operation/quarantine fact where applicable. |

## Slice-by-slice implementation sequence

Every slice begins with `git status --short`, `git rev-parse HEAD`, its
baseline tests, and a reread of the directly relevant normative sections.
Every slice ends with changed-file review, `git diff --check`, focused tests,
architecture check, formatting, vet, and one descriptive commit. No slice may
silently absorb adjacent behavior.

| Slice | Tasks | Owned behavior and stop boundary | Focused verification | Commit subject |
|---|---|---|---|---|
| P02-00 | Entry, P02-T30 foundation | Package/test-fixture skeleton, feature-branch evidence, stronger dependency test, `domain-test` and `domain-property-test` Make targets. No states or values. | Existing Go suite, architecture test, three binaries build. | `chore(domain): establish phase two purity boundaries` |
| P02-01 | T01, T05 IDs, T27/T29 | Distinct parsed UUID IDs; non-negative revision/epoch; positive attempt/wave ordinal. No lifecycle types. | `value` tests plus error-redaction cases. | `feat(domain): add canonical identifiers and bounded counters` |
| P02-02 | T03–T05, T27/T29 | UTC time, IANA zones/windows, labels, digest, firmware version, remote task URI, endpoint identity, and policy limits. No network/SSRF logic. | Boundary, DST, normalization, URI, and secret-free error tests. | `feat(domain): validate temporal identity and firmware values` |
| P02-03 | T02, T27/T29 | Strict campaign/target/artifact/inventory enum parsers and reason registry. No transition catalog or behavior. | Accepted/unknown/case/empty/round-trip/uniqueness tests. | `feat(domain): define lifecycle states and reason codes` |
| P02-04 | T06, T22 types, T29 | Bounded normalized observations and conservative ambiguity outcomes. No target mutation. | Stale/duplicate/contradictory/inconclusive/false-absence cases. | `feat(domain): model normalized update evidence` |
| P02-05 | T07, T08C, campaign T09, T21 | Complete campaign state-command Cartesian catalog and JSON fixture. No campaign reducer. | Fixture-to-catalog equivalence and one-row-per-pair completeness. | `feat(domain): specify campaign transition catalog` |
| P02-06 | T08, T08A–D, target T09 | Complete target command/evidence catalog, resume and attention rules, canonical fixture. No reducer. | Completeness, resume-step, attention-exit, and no-observation-to-submit tests. | `feat(domain): specify target update transition catalog` |
| R1 | Review only | Review enum/catalog completeness, terminal behavior, pause/resume, retry/attention/abort semantics. Do not edit behavior unless a documented defect is found. | Full catalog and architecture suite. | `docs(domain): record phase two contract review` only if evidence is added. |
| P02-07 | T13, T21 | Campaign reducer following guard order: invariant, input, duplicate, revision, epoch, catalog/safety, result. No target reducer. | Every catalog row, stale/duplicate, immutability, revision/epoch, pause/resume, terminal tests. | `feat(domain): implement campaign transition reducer` |
| P02-08 | T14, T18, T20–T22, T27/T28 | Target-update reducer with monotonic evidence and controlled submission intent. No adapter, queue, reservation table, or lease. | Catalog rows, reordered evidence, retry steps, ambiguity/attention, terminal and no-second-submit tests. | `feat(domain): implement target update safety reducer` |
| P02-09 | T10, T16, INV-04 | Wave ordinal/lifecycle/evaluation and release decision. No candidate query or planning. | First/duplicate/backward/skipped ordinal and immutable evaluation tests. | `feat(domain): enforce sequential wave release eligibility` |
| P02-10 | T17–T20 | Integer budget, retry classification, maintenance windows, and success policy as independent pure decisions. No timers, queues, or probes. | Equality/zero denominator, attempt exhaustion, DST boundaries, health/version freshness cases. | `feat(domain): add deterministic rollout safety policies` |
| R2 | Review only | Audit reducer guard ordering, immutable values, epoch safety, and budget boundary semantics. | Reducer/race/domain checks plus fixture review. | `docs(domain): record reducer safety review` only if evidence is added. |
| P02-11 | T15 | Deterministic canary seam: frozen candidates, normalized domains, algorithm version, seed/order. No database selector or final stratifier. | Order independence, duplicates, insufficiency, version, provenance/uniqueness tests. | `feat(domain): define deterministic canary selection contract` |
| P02-12 | T11–T12 | Artifact readiness and target administrative availability types. No S3/discovery/preflight calls. | Lifecycle/terminal/parser/eligibility separation tests. | `feat(domain): model artifact and target administration states` |
| P02-13 | T23, T24, T26 events | Stable event names/payloads and event fixture contracts. No outbox/publisher. | Canonical JSON, ordering, revision/reason, bounded-size, secret-canary tests. | `feat(domain): define versioned domain event contracts` |
| P02-14 | T25, T26 replay | Versioned pure replay with deterministic results and golden traces. No event store or callback. | Repeated byte-stable replay, golden, unknown-version fail-closed, input-order safety tests. | `feat(domain): add deterministic decision replay` |
| P02-15 | Model requirement | Independent reference model and generators using one pinned test dependency; it may not call production reducers/catalog lookups. | Impossible-state model tests and a deliberate local mutation-detection proof. | `test(domain): add independent sequential reference model` |
| P02-16 | PT-01,02,03,06,07,10 | Campaign/wave property suite with 200 sequences × 100 actions in PR budget. | Seeds, shrinking, transition coverage, runtime report. | `test(domain): verify campaign and wave state properties` |
| P02-17 | PT-04,05,08,09,11 | Target/retry/attention/replay property suite at same budget. | Seeds, shrinking, golden/model agreement, no-submit observation proof. | `test(domain): verify target retry and replay properties` |
| R3 | Review only | Prove oracle independence and retain an unsafe-mutation detection result. | Property suite and mutation evidence. | `docs(domain): record model oracle review` only if evidence is added. |
| P02-18 | T27–T30 defensive | Parser/decoder fuzz corpus, invariant assertions, bounded errors, and secret-safety checks. No raw Redfish fuzzing. | Fixed fuzz seed/corpus, no panic, dependency report. | `test(domain): harden domain contract boundaries` |
| P02-19 | Exit criteria | Documentation/evidence/traceability integration only. Behavior change requires separately justified correction commit. | Full aggregate, Linux CI race, coverage, import, fixture, and evidence checks. | `docs(domain): record phase two verification evidence` |
| R4 | Review only | Trace every task/property/invariant to code or explicit later-phase dependency. | Full Phase 02 exit gate. | `docs(domain): close phase two contract review` |

## Reducer guard order

Campaign and target-update reducers use the same safety sequence. No command
may skip an earlier guard:

1. Validate aggregate structural invariant and all bounded input values.
2. Resolve an explicitly supplied prior semantic command result as stable replay/no-op.
3. Reject stale expected revision.
4. For submission-relevant work, reject stale campaign epoch before producing an admission intent.
5. Look up the complete transition-catalog row; unknown combinations reject deterministically.
6. Evaluate row-specific guards from explicit policy/evidence facts.
7. Return a copied resulting aggregate, exactly one revision increase,
   reason/event/counter/next-intent values, or a stable unchanged decision.

Target evidence handling is monotonic: duplicate/older evidence may be a
no-op, but it cannot regress a terminal target or recreate a submit intent.
`NEEDS_ATTENTION` leaves only three exits: adopt matching operation, terminal
failure, or independently proof-backed retry.

## Transition catalog requirements

The catalog is the source reviewed before reducers exist. It must use
machine-readable versioned fixtures and compile into a strict lookup without
default/fallthrough transitions.

| Aggregate | Cartesian domain | Required per allowed row |
|---|---|---|
| Campaign | every campaign state × every campaign command | guard set, output state, revision/epoch effect, suspended phase behavior, reason, event, next intent |
| Target update | every target state × command/evidence family | guard set, output state, retry resume step, ambiguity/attention behavior, counter/reservation effect, reason, event, next intent |
| Wave | lifecycle/evaluation/ordinal decision set | exact predecessor ordinal, evaluation predicate, immutable terminal behavior |

Every unsupported pair is present as a rejected/no-op row; it is never an
implicit missing map key. JSON fixtures detect changes to state names,
commands, fields, casing, and reason/event values.

## Testing and CI plan

### Per-slice gate

```text
gofmt -w only the changed Go files, then confirm no formatting diff
go vet ./internal/domain/...
go test -count=1 ./internal/domain/...
go test -count=1 ./internal/architecturetest/...
git diff --check
```

When a property package exists, include `go test -count=1 ./test/domainmodel/...`.
For reducers/properties, also run the equivalent `-race` command locally when
the toolchain supports it; otherwise record the host compiler limitation and
require the Linux GitHub Actions race result. The final slice additionally runs
`make verify`, all three role builds, the full Go suite, fixed fuzz corpus, and
the CI workflow. Docker is excluded by project-owner decision.

### Makefile additions

P02-00 may add only compositional aliases:

- `domain-test`: `go test -count=1 ./internal/domain/... ./internal/architecturetest/...`
- `domain-property-test`: runs `./test/domainmodel/...` when that package
  exists; before P02-15 it must be a documented no-op-free target that reports
  there is no property package rather than masking an absent suite.
- `domain-fuzz-seed`: runs the retained bounded fuzz corpus, never an
  unbounded developer-machine fuzz session.

The existing `verify` target remains the aggregate Phase 01 gate and is only
expanded when a new domain test is already included by `go test ./...`.

### CI policy

- Update path filters so changes under `internal/domain/**`, `test/domainmodel/**`,
  and `test/fixtures/domain/**` run the Go job.
- Keep `dependency-review` pull-request-only. It is an intentional workflow
  condition, not a skipped domain test.
- Keep CI non-containerized for this phase.
- Upload Go test output on failure exactly as Phase 01 does; do not suppress
  result artifacts or retry flaky safety tests.

## Invariant traceability

| Invariant | Phase 02 domain responsibility | Later enforcement dependency |
|---|---|---|
| INV-01 one campaign per target | Expose conflict/eligibility contracts only. | Phase 03/05/07 unique constraints and readiness ownership. |
| INV-02 persist intent before side effect | Define next-intent/admission shape only. | Phase 03/09 transaction and instrumented boundary. |
| INV-03 no blind/late repeat | Conservatively classify ambiguity and prohibit inconclusive retry. | Phase 04/09 independent simulator proof and late-caller exclusion. |
| INV-04 wave ordering | Enforce prior ordinal terminal+successful evaluation. | Phase 07 transactional release. |
| INV-05 epoch admission fence | Reducer rejects post-epoch admissions; preserves earlier in-flight exposure. | Phase 08/09 persisted transaction and worker race proof. |
| INV-06 snapshot immutability | Value-copy/frozen-policy contracts only. | Phase 03/05/06/07 persistence/application enforcement. |
| INV-07 audit with mutation | Emit sufficient event/decision facts only. | Phase 03/10 same-transaction audit storage. |
| INV-08 queue is not truth | Replay and deterministic next intents only. | Phase 03/08/09 due work/outbox/recovery. |
| INV-09 version + health success | Enforce full success policy. | Phase 04/09 evidence adapters and contradictory fixtures. |
| INV-10 secret exclusion | Bound values/errors/events and test secret canary exclusion. | Phase 01/06/10/11/12 storage, telemetry, and UI controls. |
| INV-11 no DB lock during network | No DB/network code exists in domain. | Phase 03/05/08/09 instrumentation. |
| INV-12 finite correct-step retries | Enforce budget and explicit resume-step policy. | Phase 08/09 scheduling and device recovery. |
| INV-13 physical concurrency | Represent no false release on ambiguity only. | Phase 08/09 durable operation reservation. |
| INV-14 target ownership | Represent administrative/update separation only. | Phase 03/05/07/09 enrollment/quarantine. |

## Evidence, review, and rollback

The closing evidence package records branch and commit SHAs, Go/property library
versions, state inventory, catalog coverage, reason registry, unit/property/
fuzz results, seeds and runtimes, retained counterexamples, deliberate-mutation
proof, replay golden equality, Linux race result, dependency report, focused
coverage, invariant map, and known limitations.

Stop immediately and return for a design decision if a normative state or
invariant conflicts, a rule requires hidden I/O/randomness, a retry lacks an
exact resume step, absence cannot be independently proven, pause is treated as
cancellation of a prior admission, an oracle reuses production reduction logic,
or a test is flaky. Roll back only the single coherent slice commit after
retaining its failing seed/fixture; never reset the feature branch or discard
unrelated user work.

## Commit and push policy

- One accepted slice equals one descriptive conventional commit from the table.
- Review-only checkpoints produce a commit only when they add durable evidence;
  do not create empty bookkeeping commits.
- Before every commit: inspect `git status --short`, changed files, and
  `git diff --check`; never stage unrelated content.
- Push the feature branch after each successful coherent commit so CI validates
  that checkpoint. If a CI job is intentionally skipped by path/event policy,
  record why rather than calling the slice fully validated.
- Do not merge to `main` or open/deploy cloud resources in this phase without a
  separate owner request.

## Implementation approval gate

Before code changes begin, the project owner must approve this sequence and
the frozen policy choices above—especially inclusive failure-budget equality,
zero-denominator no-release, skipped/cancelled denominator treatment, and the
strict conservative ambiguity rule. The implementation then begins only with
P02-00 and stops after its commit, tests, push, and report.
