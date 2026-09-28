# ADR-021 — Deterministic lock order for persistence writers

- **Date:** 2026-09-14
- **Status:** Accepted early for Phase 03; owner phase 08

## Context

Readiness, admission, pause/abort, evaluation, completion, and repair overlap
on campaigns, target ownership, admissions, reservations, counters, and due
work. A per-query lock choice without a total order creates deadlock and stale
writer risk.

## Decision

Every multi-row persistence writer acquires rows in this total order, sorting
within each family by stable UUID/subject key:

1. campaign aggregate and command-epoch rows;
2. campaign-target and target-update rows;
3. target-enrollment rows;
4. global, site, and rack scope-capacity rows in that order;
5. submission-admission rows;
6. operation-reservation rows;
7. inventory, artifact, and frozen-preflight rows;
8. due-work and outbox rows;
9. audit and decision-input append rows.

Writers skip row families that are not part of their operation; skipped rows do
not permit reordering of the remaining families. The query catalog repeats this
sequence verbatim and must be updated with this ADR.

Inventory and artifact writers lock their aggregate before any associated
frozen-preflight row. Readiness locks its campaign before sorted targets and
enrollment claims, then candidate/artifact evidence in the listed order.
Admission locks campaign, target update, enrollment, scope capacity, admission,
and reservation in the listed order. Pause/abort locks the campaign first.
Evaluator, result, and repair paths reload the campaign/target state and obey
the same family ordering. A deadlock or serialization SQLSTATE is retried only
at a safe outer transaction boundary with a small bounded budget.

## Consequences

- Writers share a reviewable lock protocol before Phase 08 queue execution.
- A work-lease expiry cannot release a durable reservation or enrollment.
- Integration history tests can force overlaps and assert safe retries.

## Failure and rollback

Unexpected lock order is a correctness defect, not a performance tuning issue.
Transactions are kept short and contain no network calls. Contention that
exhausts the retry budget returns a classified unavailable/conflict result and
preserves all durable evidence.

## Required evidence

- Transaction table and query annotations name the acquired row families.
- Barrier tests cover readiness, admission, abort, result, and repair overlap.
- Concurrency history runs record SQLSTATE, retry count, order, and invariants.
