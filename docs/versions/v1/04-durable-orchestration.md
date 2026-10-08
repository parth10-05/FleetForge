# V1-P04 — Durable Queue Orchestration

## Goal

Use a provider-neutral at-least-once wakeup boundary only. PostgreSQL due work, leases, outbox, admissions, and durable operation reservations remain authoritative through duplicate, delayed, lost, or uncertain delivery. The local deterministic adapter proves controller behavior; live SQS and EventBridge integration is a v2 provider gate.

## Tasks

- [x] **V1-P04-T01** Define a versioned identifier-only queue envelope containing queue class, `command_id`, delivery generation, expected state/revision/epoch, not-before time, and trace context. Never use delivery-provider IDs as semantic identity.
- [x] **V1-P04-T02** Define publisher and receipt ports outside AWS SDK types. Reject unknown envelope/work versions to an auditable quarantine path.
- [x] **V1-P04-T03** Implement bounded oldest-first outbox claim, publish outside the transaction, and fenced result marking. Unknown publish outcome may repeat the wakeup only with the same event/command identity.
- [x] **V1-P04-T04** Configure a bounded local publisher/sweeper trigger so overdue work recovers when no worker loop is active; preserve the same port and invocation contract for the later EventBridge Scheduler adapter.
- [x] **V1-P04-T05** Validate queue payload/schema/size and claim due work with owner, unguessable lease token, expiry, expected revision, and command epoch. Reload authoritative state after claim.
- [x] **V1-P04-T06** Recheck campaign epoch, pause/abort state, maintenance window, retry budget, enrollment, reservation, and expected state before dispatching a handler.
- [x] **V1-P04-T07** Implement a bounded worker pool and handler deadline shorter than lease/shutdown deadlines. No handler sleeps for future work and no unbounded goroutines are allowed.
- [x] **V1-P04-T08** Acknowledge or delete a wakeup only after a durable result transaction commits. A failed or unknown database commit must leave delivery safely retryable.
- [x] **V1-P04-T09** Implement indexed sweeper repair: recreate missing wakeups with a new delivery generation while retaining semantic `command_id`; repair expired leases without stealing unexpired work.
- [x] **V1-P04-T10** Acquire global, site, and rack reservation rows in the accepted total lock order. Capacity wait schedules jittered durable retry; no counter or reservation releases from work-lease expiry.
- [x] **V1-P04-T11** Persist safe metrics for queue age, due-work age, lease conflict/expiry, reservation use, retry budget, and dead-letter/quarantine age.

## Required proof

- Concurrent duplicate wakeups produce one valid lease/transition; stale workers cannot commit.
- Worker crash after claim, handler result before DB commit, DB commit before queue acknowledgement, lost queue message, and publisher success before mark all recover from PostgreSQL.
- One hot rack cannot exceed its physical cap and cannot prevent eligible work in another rack from being considered.
- Wakeup-adapter outage and zero active worker loops leave durable work intact; the bounded scheduler trigger launches a repair pass after recovery.

## Exit gate

The v1 worker can drive synthetic-target-safe work without losing semantic work or exceeding durable capacity. This gate makes no live SQS or EventBridge claim. The worker still does not submit a firmware request until V1-P05 passes.
