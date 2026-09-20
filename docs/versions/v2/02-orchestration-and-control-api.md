# V2-P02 — Complete Orchestration and Control API

## Goal

Complete the durable worker and authenticated API contracts so queue delivery, service loss, manual attention, revision races, and API retries remain safe at the operator boundary.

## Tasks

- [ ] **V2-P02-T01** Complete explicit queue classes and identifier-only envelopes; reject incompatible work versions to quarantine and retain the command/delivery-generation audit trail.
- [ ] **V2-P02-T02** Complete outbox poison policy, controlled manual redrive, publisher shutdown, SQS visibility/redrive limits, worker/pool/dispatch budgets, and queue outage repair evidence.
- [ ] **V2-P02-T03** Implement remaining worker handlers for discovery, artifact verification, campaign evaluation, reconciliation, and projection refresh with one bounded handler per work type.
- [ ] **V2-P02-T04** Complete durable reservation transfer/release rules, capacity wait jitter, rate-limit/circuit-breaker seams, fairness observation, and deadlock-safe lock-order enforcement.
- [ ] **V2-P02-T05** Complete Redfish event authentication/correlation, stale-event protection, reconciliation sweeps, correlated failure detection, retry budgets, and the frozen target-specific proof-of-absence policy.
- [ ] **V2-P02-T06** Publish complete OpenAPI 3.1 source, generated binding/client rules, breaking-change checks, pagination, filtering, RFC 9457 errors, trace context, and bounded body/header/stream limits.
- [ ] **V2-P02-T07** Complete Cognito identity validation, JWKS cache behavior, subject-to-actor mapping, worker identity separation, recent-auth enforcement, permission matrix, anti-enumeration, and rate-limit failure policy.
- [ ] **V2-P02-T08** Implement all control commands with idempotency, expected revision, asynchronous operation semantics, audit behavior, and safe response replay. Handlers remain thin application adapters.
- [ ] **V2-P02-T09** Add resumable SSE as non-authoritative projection hints with authorization, bounded slow-client handling, event cursor/heartbeat, and mandatory snapshot refetch after a gap.
- [ ] **V2-P02-T10** Build full contract, auth, idempotency, revision-race, queue/crash, stale-live-worker, and no-network-under-lock test suites in CI and staging fault subsets.

## Exit gate

The complete API can safely expose all operator controls without synchronously driving devices, while duplicate delivery, stale workers, queue loss, and API races remain fenced by PostgreSQL truth and independent simulator evidence.
