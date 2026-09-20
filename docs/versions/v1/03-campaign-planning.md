# V1-P03 — Deterministic Campaign Planning

## Goal

Turn a frozen target/artifact snapshot into one immutable v1 canary-and-one-wave plan. The plan is explainable, replayable, and safe to start, but does not itself perform Redfish work.

## Tasks

- [ ] **V1-P03-T01** Define a versioned v1 policy schema: canary size, one later wave, global/site/rack limits, failure threshold, expected version/health rule, maintenance window, and retry budget. Reject omitted unsafe defaults.
- [ ] **V1-P03-T02** Document one integer threshold/rounding rule and validate cross-field limits, target count, rack labels, maintenance window, and policy versions before planning.
- [ ] **V1-P03-T03** Load the target/artifact snapshot under expected revisions and reject changed, quarantined, enrolled, incompatible, or stale inputs.
- [ ] **V1-P03-T04** Implement deterministic canary selection: stable seed plus target ID hash, rack diversity where possible, persisted algorithm version, target order, and selection explanation.
- [ ] **V1-P03-T05** Assign every frozen target exactly once to the canary or one later wave; persist effective sizes, failure-domain distribution, thresholds, and plan hash atomically.
- [ ] **V1-P03-T06** Make unchanged validation idempotent: same policy, snapshot, and seed return the same plan hash; an allowed draft revision invalidates the earlier plan.
- [ ] **V1-P03-T07** Atomically acquire enrollment for every target while changing the campaign to `READY`. Overlapping campaigns must produce exactly one winner.
- [ ] **V1-P03-T08** Implement start as one epoch/revision-aware command that releases only canary due-work intent with audit evidence; duplicate start returns the stable original result.
- [ ] **V1-P03-T09** Implement canary evaluation from terminal outcome counts and freshness-bounded health observations. Missing or stale evidence cannot pass evaluation.
- [ ] **V1-P03-T10** Implement later-wave release atomically with the evaluation decision; `NEEDS_ATTENTION`, a budget breach, or unresolved operation pauses the campaign and blocks new admissions.
- [ ] **V1-P03-T11** Persist plan, validation, start, evaluation, pause, and replay explanations with stable reason codes and policy/reducer versions.

## Required proof

- Reordered input and repeated seed produce the same plan; generated small fleets have no duplicate or missing assignment.
- Two overlapping readiness attempts yield one `READY` campaign; duplicate start yields one canary intent.
- Boundary failure counts follow the documented rule, no later wave releases before valid canary evaluation, and a degraded health observation pauses the campaign.

## Exit gate

v1 can create, validate, explain, freeze, start, pause, and replay a small deterministic campaign plan. No queue worker may submit firmware until V1-P04 and V1-P05 pass.
