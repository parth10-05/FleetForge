# ADR-011 — Durable, lease-owned idempotency claims

- **Date:** 2026-09-14
- **Status:** Accepted
- **Owner phase:** 03 and 10

## Context

Mutating requests can be delivered concurrently or retried after a timeout.
HTTP-local idempotency cannot safely recover a process crash or decide whether a
command already committed.

## Decision

Persist a unique claim keyed by normalized actor, normalized route, and a hash
of the idempotency key. Persist a canonical request hash separately. The first
caller creates an `IN_PROGRESS` claim with a bounded ownership lease in a short
transaction. An identical concurrent caller receives an explicit in-progress
response with retry guidance; it never waits unboundedly. A completed claim
replays its stored stable response reference. A different request hash always
conflicts.

The aggregate mutation and idempotency completion occur together in a second
short transaction. Expired ownership requires inspection of command identity
and durable result before recovery; it cannot bypass later side-effect
ambiguity rules.

## Consequences

- Process crashes leave durable, inspectable state.
- Clients converge on one completed result without rerunning a committed
  command.
- Claim cleanup is delayed beyond supported retry windows and never deletes an
  active owner.

## Failure and rollback

An active claim is retained on ambiguous failure and surfaced as retryable
in-progress, not silently re-executed. Recovery is an audited workflow. The
stable response reference permits replay after application restart.

## Required evidence

- Barrier tests for identical and conflicting request hashes.
- Crash-point tests before and after completion transaction commit.
- Tests proving an active claim cannot be cleaned up or reclaimed early.
