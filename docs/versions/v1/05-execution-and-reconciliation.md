# V1-P05 — Firmware Execution and Ambiguity Reconciliation

## Goal

Implement one frozen Redfish update path against the synthetic target backend with the no-blind-duplicate protocol. This is v1's highest-risk phase and requires focused commits and independent-journal fault evidence.

## Tasks

- [ ] **V1-P05-T01** Define the v1 adapter command/result contracts binding target, exact immutable object version/digest, update mechanism, attempt ID, task identity/fingerprint, and normalized safe error classes.
- [ ] **V1-P05-T02** Implement bounded precheck: revalidate target/campaign/artifact revision, expected version, capability freshness, policy, command epoch, enrollment, and reservation. Persist decision/evidence before proceeding.
- [ ] **V1-P05-T03** Treat an already active expected version as evidence-backed no-op/success only after configured health proof; never send an update merely because work was delivered.
- [ ] **V1-P05-T04** In one short admission transaction, persist `SUBMITTING` intent, unique attempt, epoch/policy/lease checks, durable operation reservation, audit, and due-work evidence. A pause committed before this transaction must block admission.
- [ ] **V1-P05-T05** Bind artifact transfer to the exact immutable object version. Stream the selected synthetic artifact only through the frozen worker-to-target transfer mechanism with byte, deadline, and cancellation limits; never through the API.
- [ ] **V1-P05-T06** Execute one bounded update request outside database locks. Persist accepted task identity and delayed observation work atomically while retaining reservation through reboot, version, and health resolution.
- [ ] **V1-P05-T07** Classify explicit pre-send failure as retryable only when transport proof establishes no write. Treat every possibly accepted timeout/reset as ambiguity, never generic retry.
- [ ] **V1-P05-T08** Implement task, inventory, reboot, version, and health observation with jittered durable scheduling, freshness rules, event-as-hint behavior, and strict `resume_step` preservation.
- [ ] **V1-P05-T09** Implement reconciliation using original attempt identity/time boundary and the frozen evidence hierarchy. Adoption reuses the original attempt; expected version continues to health; generic absence is inconclusive.
- [ ] **V1-P05-T10** Permit resubmission only after persisted `PROVEN_ABSENT`, remaining policy budget, and target-specific late-original-caller exclusion. Otherwise move to `NEEDS_ATTENTION`, retain quarantine/reservation, and pause.
- [ ] **V1-P05-T11** Implement a minimal audited admin resolution command: adopt-and-observe, terminal failure with explicit safe reservation disposition, or independently proof-backed retry. It requires an authorized admin OIDC identity, recent auth, reason, expected revision, and idempotency.
- [ ] **V1-P05-T12** Add deterministic fault schedules and an independent checker that joins simulator received/accepted journal records to FleetForge attempt/admission records.

## Required proof

- Duplicate submit delivery produces one attempted network call.
- Pre-write failure may retry only under durable proof; post-accept disconnect, worker death, and DB loss after acceptance create adoption or attention, never blind resubmission.
- A stalled original worker after lease expiry cannot create a later second admission/call; reservation and quarantine remain active.
- Task loss, lost/duplicate/out-of-order event, version contradiction, health failure, pause-before-admission, and pause-after-admission follow the frozen state and evidence rules.
- The simulator test fails deliberately for a duplicate request and for a false `PROVEN_ABSENT` record.

## Exit gate

For every v1 fault schedule, the independent simulator journal reports at most one accepted operation per intended update. Every unresolved attempt remains visible as adopted, proven absent under the narrow rule, terminal evidence, or `NEEDS_ATTENTION`.
