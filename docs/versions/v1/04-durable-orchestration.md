# V1-P04 — Durable Queue Orchestration

## Goal

Use SQS as at-least-once delivery only. PostgreSQL due work, leases, outbox, admissions, and durable operation reservations remain authoritative through duplicate, delayed, lost, or uncertain delivery.

## Tasks

- [ ] **V1-P04-T01** Define a versioned identifier-only queue envelope containing queue class, `command_id`, delivery generation, expected state/revision/epoch, not-before time, and trace context. Never use SQS IDs as semantic identity.
- [ ] **V1-P04-T02** Define publisher and receipt ports outside AWS SDK types. Reject unknown envelope/work versions to an auditable quarantine path.
- [ ] **V1-P04-T03** Implement bounded oldest-first outbox claim, publish outside the transaction, and fenced result marking. Unknown publish outcome may repeat the wakeup only with the same event/command identity.
- [ ] **V1-P04-T04** Configure the EventBridge Scheduler bounded publisher/sweeper task so overdue work can recover when worker desired capacity is zero.
- [ ] **V1-P04-T05** Validate queue payload/schema/size and claim due work with owner, unguessable lease token, expiry, expected revision, and command epoch. Reload authoritative state after claim.
- [ ] **V1-P04-T06** Recheck campaign epoch, pause/abort state, maintenance window, retry budget, enrollment, reservation, and expected state before dispatching a handler.
- [ ] **V1-P04-T07** Implement a bounded worker pool and handler deadline shorter than lease/shutdown deadlines. No handler sleeps for future work and no unbounded goroutines are allowed.
- [ ] **V1-P04-T08** Acknowledge SQS only after a durable result transaction commits. A failed/unknown database commit must leave delivery safely retryable.
- [ ] **V1-P04-T09** Implement indexed sweeper repair: recreate missing wakeups with a new delivery generation while retaining semantic `command_id`; repair expired leases without stealing unexpired work.
- [ ] **V1-P04-T10** Acquire global, site, and rack reservation rows in the accepted total lock order. Capacity wait schedules jittered durable retry; no counter or reservation releases from work-lease expiry.
- [ ] **V1-P04-T11** Persist safe metrics for queue age, due-work age, lease conflict/expiry, reservation use, retry budget, and dead-letter/quarantine age.

## Required proof

- Concurrent duplicate SQS receipts produce one valid lease/transition; stale workers cannot commit.
- Worker crash after claim, handler result before DB commit, DB commit before queue acknowledgement, lost queue message, and publisher success before mark all recover from PostgreSQL.
- One hot rack cannot exceed its physical cap and cannot prevent eligible work in another rack from being considered.
- Queue outage and zero worker capacity leave durable work intact; the scheduler launches a bounded repair pass after recovery.

## Exit gate

The v1 worker can drive simulator-safe work without losing semantic work or exceeding durable capacity. The worker still does not submit a firmware request until V1-P05 passes.
