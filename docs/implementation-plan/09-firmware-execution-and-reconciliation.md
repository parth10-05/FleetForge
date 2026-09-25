# Phase 09 — Firmware Execution and Ambiguity Reconciliation

## Outcome

Implement the safety-critical Redfish firmware workflow: bounded precheck, persisted submission admission, update submission, remote-task tracking, reboot observation, firmware/version verification, health verification, and ambiguity reconciliation. Prove with an independent device journal that duplicate delivery, worker failure, and an original worker that outlives its lease cannot cause a blind or late duplicate firmware submission.

This is the flagship phase. It should receive the strongest review and fault testing.

## Entry criteria

- Phases 04–08 pass.
- The accepted synthetic target contract defines one update mechanism,
  timing behavior, artifact data path, body limits, selected Redfish-shaped
  resources, task/inventory evidence, faults, and recovery procedure.
- Simulator supports pre-accept and post-accept connection faults, task loss, reboot, contradictory success, and health failure.
- Artifact URL/access strategy from worker/device is accepted.
- `INV-02`, `INV-03`, `INV-05`, `INV-09`, `INV-11`, and `INV-12` have executable tests planned.

## Non-goals

- No support for every vendor update mechanism.
- No claim of true exactly-once external execution.
- No force-cancel assumption for accepted synthetic-target update tasks.
- No automatic retry after signature/incompatibility/explicit terminal rejection.
- No real production hardware flashing.

## Happy-path sequence

```mermaid
sequenceDiagram
  participant W as Worker
  participant DB as PostgreSQL
  participant R as Redfish adapter
  participant B as synthetic target backend

  W->>DB: claim PRECHECK and reservation
  W->>R: bounded capability/current-version check
  R->>B: GET UpdateService + SoftwareInventory
  B-->>R: eligible/current version
  W->>DB: commit SUBMITTING intent + attempt ID
  W->>DB: persist epoch-fenced admission + durable operation reservation
  W->>R: submit exact artifact operation
  R->>B: update action
  B-->>R: accepted + task URI
  W->>DB: persist remote identity + UPDATE_ACCEPTED; retain operation reservation
  W->>DB: schedule delayed task observation
  W->>R: poll task/inventory after delay
  R->>B: task/inventory/health requests
  B-->>R: reboot then expected version + health
  W->>DB: commit VERIFIED/SUCCEEDED + audit/counters
```

## Ambiguous-window sequence

```mermaid
sequenceDiagram
  participant W1 as Original worker
  participant DB as PostgreSQL
  participant B as Synthetic target
  participant W2 as Recovery worker

  W1->>DB: commit SUBMITTING attempt A + admission + operation reservation
  W1->>B: submit update A
  B-->>W1: accepts, but connection/worker is lost
  Note over W1,DB: no durable remote task URI
  W2->>DB: claim expired work lease; transfer reconciliation ownership without releasing operation reservation
  W2->>B: query task/update service/inventory/health
  alt matching task or expected version
    W2->>DB: adopt operation and continue observation
  else side effect proven absent and late original request excluded
    W2->>DB: mark retry eligible within budget
  else contradictory or inconclusive
    W2->>DB: NEEDS_ATTENTION and pause per policy
  end
  Note over W2,B: never submit again before proof of absence
```

## Evidence hierarchy

Recommended precedence for reconciliation:

1. Matching durable remote operation/task identity with compatible request fingerprint.
2. Expected firmware version/activation state observed after the attempt's time boundary.
3. UpdateService or software inventory indicates matching staged/activating image.
4. Explicit remote task rejection/failure tied to the attempt.
5. Strong proof that no operation was accepted and no original caller can still submit, only where protocol/target behavior supports such proof.
6. Otherwise inconclusive → `NEEDS_ATTENTION`.

Absence of a task URI, staged image, or version change alone is never proof that no update occurred. An expired lease is also never proof that the original worker did not or cannot still write the request.

## Step deadlines and scheduling

| Step | Network behavior | Next action |
|---|---|---|
| Precheck | Short bounded reads | Submit intent, retry wait, or terminal result |
| Submit | One side-effecting request under a persisted admission and durable operation reservation | Persist remote identity, or enter ambiguity reconciliation; reservation remains active |
| Task observe | Short task read | Delayed observe, reboot wait, verify, failure, or ambiguity |
| Reboot wait | Reachability/inventory probe | Delayed probe; never keep request open |
| Verify version | Software inventory read | Health check, retry wait, terminal contradiction |
| Verify health | Configured bounded checks | Success, retry wait, or terminal/pause result |
| Ambiguity reconcile | Bounded evidence query set | Adopt, proven absent with late-caller exclusion, contradicted, or `NEEDS_ATTENTION`; transfer rather than expire reservation ownership |

## Micro-task checklist

### Adapter command contracts

- [ ] **P09-T01** Define update submission input binding exact target, artifact digest/version ID, update mechanism, and attempt ID.
- [ ] **P09-T02** Define adapter result classes: accepted with identity, explicit rejection, proven pre-send failure, ambiguous transport, malformed response.
- [ ] **P09-T03** Capture safe response fingerprint, task URI, message IDs/codes, and timestamps without raw secrets/firmware.
- [ ] **P09-T04** Define task observation and normalized terminal/progress states.
- [ ] **P09-T05** Define firmware inventory/activation and health observations with freshness/source.
- [ ] **P09-T06** Add an explicit target-contract strategy boundary; the V1
  path remains synthetic, narrow, and contract-tested.
- [ ] **P09-T06A** Implement the frozen artifact transfer mechanism. For push upload, stream the exact S3 version ID through the worker to the synthetic target with byte/deadline bounds; do not route bytes through the Control API.

### Precheck

- [ ] **P09-T07** Confirm target/campaign/artifact revisions and eligibility.
- [ ] **P09-T08** Refresh capabilities/current version if snapshot freshness policy requires it.
- [ ] **P09-T09** Treat already-running expected version as policy-defined no-op/success with evidence, not a new update.
- [ ] **P09-T10** Reject incompatible/unverified artifact, disabled target, stale command epoch, or missing reservation.
- [ ] **P09-T11** Persist precheck evidence and decision.

### Submission safety protocol

- [ ] **P09-T12** Commit `SUBMITTING` state and unique attempt before network call, then atomically persist one submission admission and durable operation reservation after rechecking epoch/policy.
- [ ] **P09-T13** Ensure attempt semantic key prevents another active submit attempt for same target/campaign/artifact step.
- [ ] **P09-T14** In one admission transaction, recheck lease token, expected revision, campaign epoch, pause/abort/window, failure budget, enrollment, and capacity, then persist admission. A later pause blocks new admissions but cannot revoke this one.
- [ ] **P09-T15** Bind artifact access to the exact S3 version ID. Generate target access only if the frozen mechanism supports secure pull; otherwise stream S3 to the synthetic target within the measured upload budget.
- [ ] **P09-T16** Execute exactly one update request with bounded request/body/response handling.
- [ ] **P09-T17** Persist accepted task identity/fingerprint and next observation atomically while retaining the durable operation reservation through reboot/version/health resolution.
- [ ] **P09-T18** Map explicit rejection to terminal/retry policy only from normalized Redfish evidence.
- [ ] **P09-T19** Map any possibly accepted transport failure to ambiguity path, never generic retry.
- [ ] **P09-T19A** Map work-lease expiry or handler shutdown after submission admission to reconciliation-only. It cannot create retry permission or release physical capacity.

### Task/reboot/version/health observation

- [ ] **P09-T20** Poll remote task with jittered schedule and absolute operation deadline.
- [ ] **P09-T21** Accept authenticated Redfish event only as acceleration hint; correlate and then confirm authoritative observation.
- [ ] **P09-T22** Model temporary reachability loss as expected reboot where evidence supports it.
- [ ] **P09-T23** Handle task disappearance by inventory/version/activation evidence hierarchy.
- [ ] **P09-T24** Require expected version and freshness before health evaluation.
- [ ] **P09-T25** Run configured bounded health checks and store individual outcomes.
- [ ] **P09-T26** Commit success only after version and all required health rules pass.
- [ ] **P09-T27** On pause/abort, prevent new admissions and continue observing accepted, admitted, or ambiguous work; expose pre-pause admissions as potentially in flight.

### Ambiguity reconciliation

- [ ] **P09-T28** Create distinct ambiguity-reconciliation work type with higher operator visibility.
- [ ] **P09-T29** Load original attempt identity, time boundary, target/artifact fingerprints, and any partial response evidence.
- [ ] **P09-T30** Query evidence hierarchy in a deterministic, bounded order.
- [ ] **P09-T31** Adopt matching remote operation without creating a new submit attempt.
- [ ] **P09-T32** Treat expected version after attempt as success-path adoption only after health checks.
- [ ] **P09-T33** Define protocol-specific proof-of-absence requirements including exclusion of a late original request; default generic Redfish result is conservative inconclusive.
- [ ] **P09-T34** Permit resubmission only after persisted `PROVEN_ABSENT` resolution and remaining retry/policy budget.
- [ ] **P09-T35** Move inconclusive/contradictory result to `NEEDS_ATTENTION`, retain target quarantine and durable operation reservation, and pause. Release occurs only after a defined safe evidence or audited resolution condition.
- [ ] **P09-T36** Provide manual resolution commands: adopt evidence and resume observation, mark terminal failure with an explicit reservation-release decision, or authorize retry after independent new proof; every action admin-only, recent-authenticated, and audited in Phase 10.

### Retry and systemic failure protection

- [ ] **P09-T37** Classify retryable reads separately from update submission.
- [ ] **P09-T38** Apply per-step, target, campaign, and failure-domain retry budgets.
- [ ] **P09-T39** Use full-jitter exponential backoff and cap next delay/deadline.
- [ ] **P09-T40** Detect correlated transport/device failures and pause release before storm amplification.
- [ ] **P09-T41** Ensure circuit breaker does not convert ambiguous submissions into retryable failures.

### Events and reconciliation sweep

- [ ] **P09-T42** Authenticate callback, enforce size/schema limits, and deduplicate event ID.
- [ ] **P09-T43** Record observed and received timestamps plus sequence when available.
- [ ] **P09-T44** Prevent stale/out-of-order event from regressing target state.
- [ ] **P09-T45** Schedule overdue observation/ambiguity work from PostgreSQL regardless of event presence.
- [ ] **P09-T45A** Use retry `resume_step` so task/reboot/version/health observation failures return only to their observation step and can never fall through to `SUBMITTING`.
- [ ] **P09-T46** Quarantine malformed/unknown event versions and alert without blocking polling correctness.

## Mandatory fault matrix

| Test ID | Injected point | Required result |
|---|---|---|
| P09-FT-01 | Duplicate submit work before call | One attempt/one call |
| P09-FT-02 | Worker crashes after `SUBMITTING` commit, before admission/call | Recovery reconciles; retry requires durable pre-write proof or target-specific late-caller exclusion, otherwise attention |
| P09-FT-03 | Connection fails before request write | Classified proven pre-send; bounded retry |
| P09-FT-04 | Target accepts then connection drops | One remote operation; reconciliation adopts or `NEEDS_ATTENTION`; no blind resubmit |
| P09-FT-05 | Target accepts then worker dies before DB result | Same as FT-04 |
| P09-FT-06 | Accepted task event duplicated | One logical progression |
| P09-FT-07 | Event lost | Polling completes correctly |
| P09-FT-08 | Event arrives after terminal state | Stored/deduped as evidence; no regression |
| P09-FT-09 | Task disappears after reboot, version correct | Continue to health and success |
| P09-FT-10 | Task says success, version unchanged | Terminal/attention outcome, never success |
| P09-FT-11 | Version correct, health fails | Failure/pause per policy |
| P09-FT-12 | Continuous `429/503` on observations | Bounded jitter and budget; no synchronized storm |
| P09-FT-13 | Pause commits before submission-admission transaction | Admission is rejected and no call occurs |
| P09-FT-14 | Abort while update accepted | No force-cancel assumption; observation continues |
| P09-FT-15 | Database unavailable after remote accept | Queue redelivery enters ambiguity reconciliation |
| P09-FT-16 | Conflicting inventory/task evidence | `NEEDS_ATTENTION`; evidence retained |
| P09-FT-17 | Original worker is admitted, stalls past work-lease expiry, recovery observes absence, then original resumes | No second admission/call; operation reservation remains counted; result adopts/reconciles or enters attention |
| P09-FT-18 | Pause commits after admission but before request write | Earlier admission remains visible/potentially in flight; no new admissions occur; documentation does not claim the network call is revoked |
| P09-FT-19 | Observation step enters retry wait | `resume_step` returns to the same observation family and no submit call is reachable |
| P09-FT-20 | Target update/campaign becomes terminal while operation remains unresolved | Enrollment quarantine and operation reservation block target reuse until safe resolution |
| P09-FT-21 | Restored database predates an accepted simulator operation | Recovery mode disables submission and reconciles newer persistent device state before enabling work |

## Proof of zero blind duplicate submissions

Instrument the simulator with a per-target accepted-operation journal. Run randomized schedules containing duplicate queue deliveries, injected worker termination at every persistence/network boundary, database failures, delayed responses, and lost events. The assertion is not merely “final state succeeded”; it is:

- every received request and accepted operation is recorded by the simulator independently before response-loss injection;
- accepted operation count per intended target/campaign/artifact update is at most one; a FleetForge `PROVEN_ABSENT` record does not relax this assertion;
- every `PROVEN_ABSENT` resolution agrees with the independent device journal and excludes a late original caller;
- every ambiguous schedule ends in adopted operation, proven absence, or `NEEDS_ATTENTION`;
- no generic retry path invokes submission from an ambiguous state.
- a deliberate duplicate request and a false `PROVEN_ABSENT` classification make the harness fail.

## Observability introduced

- Redfish call duration/status/failure class by operation.
- Target updates by state and step.
- Submission attempts, accepted operations, ambiguous outcomes, adoptions, proven-absent resolutions, and manual-attention count.
- Reboot wait, task age, verification age, reconciliation lag.
- Failure/retry budget remaining and correlated failure rate.
- Trace spans linking attempt, task identity hash, target update, and campaign without secret/high-cardinality metric labels.

## Evidence artifacts

- Happy and ambiguous sequence traces.
- Full fault-matrix report with simulator seeds.
- Accepted-operation journal proving no blind duplicates.
- Example `NEEDS_ATTENTION` operator evidence bundle.
- Lost-event and task-disappearance recovery transcripts.
- Selected schema limitations, simulator extensions, and external-device behavior that remains untested.

## Exit criteria

- Happy path reaches success only with version+health evidence.
- Every mandatory fault test passes.
- No ambiguous state can transition directly to a new submission.
- Pause admission-fence tests pass before and after admission without claiming that an earlier admission is externally revoked.
- Lost/duplicate/out-of-order events do not break correctness.
- Retry storms are bounded by multiple scopes.
- Accepted in-flight work remains observed during pause/abort.
- Evidence is sufficient to explain every terminal/attention outcome.

## Rollback and stop conditions

- Stop immediately if any crash schedule produces two accepted operations without a persisted proof-of-absence retry.
- Disable update submission and retain inventory/preflight mode if ambiguity handling is incomplete.
- Do not weaken safety to make a synthetic scenario convenient; add explicit
  evidence or retain manual attention.
