# V1-P05 reservation disposition completion plan

## Objective

Close fault-matrix case P05-F20 without weakening the physical-operation
reservation invariant: a target operation that may still run must continue to
consume capacity.  A terminal workflow result alone is not proof that it is
safe to release capacity.

## Current boundary

`ExecutionAdapter.ResolveAttention` atomically records an administrator's
resolution, updates the firmware attempt and target update, and optionally
schedules observation/retry work.  Its `TERMINAL` transition currently leaves
the admission's three reservations `ACTIVE`; there is no writer for
`RELEASED`, `QUARANTINED`, `released_at`, or `release_reason`.

That is safe but incomplete: capacity can never be returned after a safe,
audited terminal disposition.  It also makes reservation disposition absent
from the admin resolution's durable evidence.

## Design

Extend `execution.AdminResolutionCommand` with a closed reservation
disposition, accepted only for `TERMINAL`:

| Action | Disposition | Result |
| --- | --- | --- |
| `TERMINAL` | `QUARANTINE` | Mark all currently active admission reservations `QUARANTINED`; capacity remains consumed and target enrollment remains active. |
| `TERMINAL` | `RELEASE` | Release all three admission reservations only after an explicit administrator-provided safe-disposition proof reference. |
| `ADOPT`, `RETRY_PROVEN_ABSENT` | empty | Reject any disposition; existing observation/retry rules remain unchanged. |

`RELEASE` must require a non-empty proof reference that is persisted in the
resolution evidence.  It is not inferred from a target-update terminal state,
lease expiry, or an administrator's reason string.  The command remains
expected-revision/epoch fenced and idempotent by the complete immutable
resolution request.

## Transactional changes

Within the existing serializable `ResolveAttention` transaction, after locking
the attempt/target update and before final commit:

1. Validate action/disposition/proof coherence before writing state.
2. Lock every `ACTIVE`/`QUARANTINED` reservation for the admission in stable
   `scope_level, scope_key, id` order.
3. For `QUARANTINE`, update only `ACTIVE` reservations to `QUARANTINED`.
   Counters remain unchanged because quarantined reservations still consume
   capacity.
4. For `RELEASE`, update only non-released reservations to `RELEASED`, write
   `released_at` and a constrained `release_reason`, and decrement each
   corresponding `scope_capacities.active_reservation_count` in the same
   ordered transaction.  A zero-row counter update is a fence failure.
5. Persist the chosen disposition and release proof in
   `ambiguity_resolutions.evidence_summary`, alongside the immutable action,
   principal, reason, and proof reference.
6. Retain idempotency: a repeat of the same command returns the stable result;
   any different disposition or proof conflicts.

No target I/O occurs in this transaction.  No worker gains submission
authority.  Normal success-path release is deliberately out of scope here;
this closes only explicit administrator-controlled terminal disposition.

## Affected surfaces

- `internal/application/execution/ports.go`: closed disposition field on the
  command.
- `internal/persistence/postgres/execution_reconciliation.go`: validation,
  durable evidence, reservation mutation, and exact counter accounting.
- `internal/application/execution/resolve_test.go`: command validation unit
  coverage.
- `test/integration/postgres/`: real PostgreSQL tests for quarantine, safe
  release, stale fence rejection, double-release idempotency, and capacity
  counter integrity.
- `docs/evidence/v1-p05/fault-matrix.json`: P05-F20 moves only after the
  real PostgreSQL proof passes.

## Required proof

The integration proof must create an admitted ambiguous attempt and then show:

1. `TERMINAL/QUARANTINE` results in `FAILED_TERMINAL`, keeps all reservations
   capacity-counted, and prevents a new enrollment/admission.
2. `TERMINAL/RELEASE` with a safe proof releases exactly three reservations
   and decrements global/site/rack counters exactly once.
3. A replay is stable; a changed disposition/proof or stale revision fails
   without changing counters.
4. A missing release proof is rejected and leaves reservations unresolved.

Hosted exact-SHA PostgreSQL CI remains the final authority for the proof.
