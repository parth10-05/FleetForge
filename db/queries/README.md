# db/queries — Named SQL Access Patterns

Each file contains exactly one named query. Query IDs (P03-Qnn) link here from
migration index comments. No file contains seed data or ad-hoc column lists.
Generated sqlc code (when added) must remain inside `internal/persistence/postgres`
and must not leak into `internal/domain`.

| ID | File | Access pattern | Index used |
|---|---|---|---|
| P03-Q01 | `q01_load_campaign_waves.sql` | Load campaign waves in release order | `campaign_waves_campaign_ordinal_index` |
| P03-Q02 | `q02_find_target_campaign_membership.sql` | Find target campaign memberships | `campaign_targets_target_index` |
| P03-Q03 | `q03_load_target_update_by_campaign_target.sql` | Load target update by campaign target | `target_updates_campaign_target_index` |
| P03-Q04 | `q04_check_active_enrollment.sql` | Find active enrollment for a target | `target_enrollments_one_active_target` (partial unique) |
| P03-Q05 | `q05_acquire_enrollment.sql` | Insert enrollment claim | `target_enrollments_one_active_target` |
| P03-Q06 | `q06_check_active_operation_reservations.sql` | Find unresolved operations for a target | `operation_reservations_target_active_index` |
| P03-Q07 | `q07_latest_device_observations_by_kind.sql` | Latest evidence for target and kind | `device_observations_target_kind_time_index` |
| P03-Q08 | `q08_target_update_evidence_timeline.sql` | Evidence timeline for a target update | `device_observations_update_time_index` |
| P03-Q09 | `q09_claim_outbox_events.sql` | SKIP LOCKED publisher claim | `outbox_events_publishable_index` |
| P03-Q10 | `q10_claim_due_work.sql` | SKIP LOCKED due-work claim | `due_work_claimable_index` |
| P03-Q11 | `q11_audit_timeline.sql` | Reconstruct aggregate audit timeline | `audit_events_timeline_index` |
| P03-Q12 | `q12_resolve_idempotency_key.sql` | Resolve idempotency record | `idempotency_records_actor_route_key_unique` |
| P03-Q13 | `q13_load_decision_inputs_for_replay.sql` | Replay inputs after observations expire | `decision_inputs_aggregate_revision_sequence_unique` |
| P03-Q14 | `q14_save_campaign_with_revision.sql` | Save campaign with revision guard | Campaigns PK + revision predicate |
| P03-Q15 | `q15_save_target_update_with_revision.sql` | Save target update with revision guard | Target updates PK + revision predicate |
| P03-Q16 | `q16_insert_audit_event.sql` | Append audit event in unit-of-work | `audit_events_aggregate_revision_sequence_unique` |
| P03-Q17 | `q17_insert_outbox_event.sql` | Insert outbox event in unit-of-work | Outbox PK |
| P03-Q18 | `q18_insert_due_work.sql` | Schedule due-work in unit-of-work | `due_work_semantic_unique` |

## Lock order reminder (ADR-021)

All multi-row writers acquire rows in this order:
`campaign → wave → enrollment/target → operation-reservation → aggregate → due-work`

No query file may add a `FOR UPDATE` that violates this order.
