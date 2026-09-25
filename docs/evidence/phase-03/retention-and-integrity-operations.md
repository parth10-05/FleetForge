# Phase 03 Retention and Integrity Operations

## Retention classes

| Class | Rule |
| --- | --- |
| Device observations | Eligible only after the supplied cutoff; cleanup is bounded and operates solely on observations. |
| Idempotency | Retain through every supported client retry window. No active claim is deleted. |
| Audit and decision input | Retain together for the audit lifetime; never routine-cleanup candidates. |
| Due work and outbox | Pending, claimed, unpublished, reconciliation, and quarantined records are never routine-cleanup candidates. |
| Enrollment, admission, reservation, capacity | Durable safety evidence; explicit audited resolution is required before any future archival policy. |
| Artifacts | Retention needs a later storage-policy and reference proof; this phase deletes none. |

## Operations boundary

`CleanupExpiredObservations` accepts an explicit cutoff and bounded limit. It is not a scheduler and does not delete from any other table. Integrity scans report inconsistencies; they never mutate, release leases, decrement capacity, or auto-repair. Backup/export and restore rehearsal remain Phase 03 closeout/Phase 16 evidence.
