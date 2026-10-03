# V1-P03 — Deterministic Campaign Planning Execution Plan

## Status and authority

**Status:** proposed implementation plan; no policy-dependent production code
may be started until the decision sheet below is accepted. The architecture
freeze and accepted ADRs prevail over older phase wording. This V1 slice
implements one canary and exactly one later wave on the repository-owned
synthetic target boundary. It neither submits firmware nor claims work.

The baseline is `97be3492ed7e0bd5c97762fd40b3521566838076`, which contains
the final P02 tested integration SHA `afd4ef0c3350e44c08d29048c9dbe8f7db2a4c5f`
and its CI repair. `AcquireReadinessEnrollment` remains the sole P02 trust
recheck and atomic enrollment boundary.

## Decision sheet — approval required

The following choices close the missing ADR-018/ADR-019 decisions. They are
deliberately explicit so all layers can persist and replay the same behavior.

| Topic | Proposed V1 decision |
| --- | --- |
| Policy identity | Require `fleetforge.campaign-policy/v1`; every field is present and bounded. No defaulting of safety values. Persist the original unit (`absolute` or `percent`) and the resolved count/threshold. |
| Cardinality | Candidate size is 1–10,000. Empty candidates are rejected. A one-target candidate produces a one-target canary and an empty later wave; the latter is persisted but is never released. For more than one target, canary is 1..n-1 and later wave is nonempty. |
| Canary rounding | Absolute count must be 1..n. Percent is integral 1..100 and resolves with ceiling(`n * percent / 100`), then is capped at `n-1` when `n > 1`; a 100% request for `n > 1` is rejected rather than silently changed. |
| Selection | Sort immutable candidate inputs by target UUID. Rank by SHA-256 of length-prefixed `algorithm-version`, seed bytes, and target UUID. Select the lowest-rank target from each distinct `(site,rack)` in rank order until the canary is full; then fill by global rank. Ties use target UUID. Missing site or rack blocks planning. |
| Limits | Require positive global/site/rack limits with `rack <= site <= global`; each cannot exceed its applicable cohort size. V1 does not implement strict worst-case capacity reservation, so requests for it are rejected. |
| Failure budget | Persist an exact rational `numerator/denominator` (0 <= numerator <= denominator, denominator > 0). At each canary/wave/campaign evaluation use terminal outcomes only: failure reaches the budget when `failed * denominator >= numerator * evaluated`; zero failures remain admissible. Any `NEEDS_ATTENTION`, unresolved reservation, or nonterminal member blocks advancement independently. |
| Health and time | Require at least one named health check, expected version, positive freshness, positive observation interval, positive observation window, IANA maintenance window, and positive maximum duration. Evaluation uses a persisted logical observation cut. Every required sample must be scheduled, at or before the cut, not future-dated, fresh, and passing; missing/stale/future/degraded input blocks release. Duration expiry pauses. |
| Versions and hash | Canonical UTF-8 JSON uses fixed field order, sorted arrays/maps, decimal integers, RFC3339 UTC timestamps, and SHA-256 prefixed `sha256:`. Persist `campaign-plan/v1`, `selection/v1`, `fleetforge.campaign-policy/v1`, and the existing reducer version. Unknown versions or hash/order mismatches fail closed. |
| Reason codes | Use stable codes for policy-invalid, candidate-invalidated, assignment-selected/diversity/fill, assignment-later-wave, enrollment-conflict, canary-pending, health-blocked, budget-reached, needs-attention, duration-expired, canary-passed, and later-wave-released. |

Acceptance creates ADR-018 and ADR-019 with these values before the first
policy or planning implementation card. Any change is a deliberate versioned
contract change, not an implicit code default.

## Design and data flow

1. The validator accepts a complete versioned policy and immutable P02
   candidate/artifact tuple. It validates bounded units before canonicalizing.
2. The pure planner sorts inputs, verifies every required site/rack label,
   resolves effective counts, ranks candidates deterministically, assigns each
   target exactly once, and returns assignments plus an explainable normalized
   snapshot and plan hash. It has no clock, database, queue, storage, or
   Redfish dependency.
3. Validation orchestration loads the PASSED candidate at the expected campaign
   revision, evaluates the pure plan outside transactions, and writes the
   immutable candidate-backed plan in a short transaction.
4. The READY transaction composes the persisted plan with
   `AcquireReadinessEnrollment`; it rechecks P02 trust, candidate membership,
   artifact tuple, discovery freshness, compatibility, enrollment conflicts,
   and reservations before atomically committing all claims, READY, audit,
   decision input, and outbox evidence.
5. Start applies the existing campaign reducer with expected revision/epoch and
   creates only canary due-work/outbox intent. It neither claims the work nor
   performs update I/O.
6. Evaluation reads frozen membership, terminal update state, reservations, and
   scheduled health facts at a logical cut. It either leaves evaluation pending,
   pauses safely, or atomically records the terminal canary evaluation and
   releases the one later-wave intent. Replay consumes only persisted normalized
   facts and schedules nothing.

Every writer follows ADR-021: campaign; sorted campaign target/update; sorted
enrollment; capacity; admission; reservation; inventory/artifact/candidate;
due-work/outbox; audit/decision input. Transactions are serializable or
revision-CAS guarded, have a bounded outer retry only for serialization/deadlock,
and perform no external I/O while locks are held.

## Persisted shape

Add one forward migration after `00016` only:

- `campaign_plan_snapshots`, keyed by campaign and draft revision, holds the
  candidate ID, exact artifact tuple, canonical policy/effective-plan JSON,
  seed, versions, target snapshot hash, plan hash, and append-only creation
  facts. It is immutable after insertion.
- Add `wave_id`, `cohort`, `ordinal`, and `selection_reason` to
  `campaign_targets`; constraints require exactly `CANARY` or `LATER_WAVE`, a
  positive cohort ordinal, and one campaign target per frozen candidate member.
- Extend `campaign_waves` with the fixed cohort identity and effective counters
  required for explainable evaluation. Exactly two rows exist for candidates of
  more than one target: canary ordinal 1 and later-wave ordinal 2.
- Add immutable policy/plan guards and foreign keys/checks that prevent a
  READY campaign from mutating its snapshot or assignment. Query changes keep
  all selection and membership authority in existing campaign tables.

DDL is finalized only after inspecting generated SQLC types and migration
compatibility tests. It will not introduce a parallel membership authority.

## Implementation cards

### Card 1 — Pure policy and planner (T01, T02, T04–T06)

Affected modules: `internal/domain/policy`, a new pure
`internal/domain/campaignplan` package if package boundaries remain clear, and
their unit/property/fuzz tests.

- Define explicit wire/domain policy types, version dispatch, bounded semantic
  validation, canonical byte serialization, and stable reason codes.
- Implement immutable planner input/output, deterministic ranking/diversity,
  complete assignment proof, and frozen effective values/distributions.
- Reuse `policy.EvaluateFailureBudget`; change it only with the accepted
  versioned rule and boundary tests.
- Test schema and cross-field errors; 0/1/many; missing labels; rounding;
  reordered and seed golden inputs; rack diversity/ties; no duplicate/missing
  property tests; replay/hash stability; and canonicalization fuzzing.

### Card 2 — Persisted validation and READY (T03, T05–T07, T11)

Affected modules: migration/query catalog/SQLC output, PostgreSQL campaign-plan
repository, validation application service, `enrollment.go`, and PostgreSQL
integration tests.

- Add bounded immutable snapshot/assignment persistence and query contracts.
- Load expected PASSED candidate and exact artifact tuple; run planner before
  locks; write candidate-backed campaign membership/waves/snapshot atomically.
- Compose the final readiness path with `AcquireReadinessEnrollment`, never a
  duplicate trust recheck. Validate complete frozen membership and plan hash
  before enrollment.
- Test constraints/immutability, successful six-target/two-rack readiness,
  invalidated inputs, full rollback on one blocked member, idempotency,
  overlap race, and bounded serialization recovery.

### Card 3 — Start, evaluation, pause, and later-wave intent (T08–T10)

Affected modules: campaign application service, PostgreSQL command unit of
work, due-work/outbox adapters, pure evaluator, and race tests.

- Make start revision/epoch/idempotency-aware and generate only canary work.
- Evaluate persisted terminal outcomes, reservations, health schedule, logical
  cut, budget, maintenance, and duration using frozen policy/version facts.
- Atomically write decision/audit/replay facts and either pause or one later
  wave intent. Enforce single release under concurrent evaluators.
- Test duplicate/concurrent start, start/pause barrier, exact budget boundary,
  zero failure, active exposure, attention, nonterminal, bad health, expiry,
  and no P01 journal firmware request.

### Card 4 — Replay and closeout (T11)

Affected modules: replay application/domain adapter, evidence documentation,
and component/integration test fixtures.

- Reconstruct validation/start/evaluation/pause/release from stored canonical
  facts without scheduling work; fail closed for unknown version/order/hash.
- Run unit/property/fuzz, Docker-free local Go validation, and hosted CI’s
  tagged PostgreSQL/Linux race gates. Retain exact SHA, migration, policy/seed,
  plan hash, replay transcript, DB excerpts, and independent-journal assertion.

## Verification and non-claims

Local evidence is limited to Docker-free formatting, unit/property/fuzz tests,
`go vet ./...`, builds, and diff checks. PostgreSQL integration and race proof
are CI-only per repository instructions. P03 does not claim a 1,000-target
result unless a separately measured run is explicitly performed; its target
size limit is the P02 10,000 candidate cap with no scale-performance claim.
No card may add firmware submission, Redfish I/O, queue claiming, external
provider behavior, or a new readiness trust path.
