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
| P03-Q19 | `q19_insert_decision_input.sql` | Persist reducer evidence in unit-of-work | `decision_inputs_aggregate_revision_sequence_unique` |
| P03-Q20 | `q20_lock_scope_capacity.sql` | Lock a durable scope capacity row during admission | `scope_capacities` primary key |
| P03-Q21 | `q21_complete_due_work.sql` | Fence a due-work completion by token/revision/epoch | due-work primary key |
| P03-Q22 | `q22_fail_due_work.sql` | Fence a due-work retry by token/revision/epoch | due-work primary key |
| P03-Q23 | `q23_repair_expired_due_work.sql` | Select expired due-work leases for bounded repair | `due_work` expiry claim index |
| P03-Q24 | `q24_repair_expired_outbox.sql` | Select expired outbox claims for bounded repair | `outbox_events_expired_claim_index` |
| P03-Q25 | `q25_oldest_due_work.sql` | Inspect oldest pending due work | `due_work_oldest_pending_index` |
| P03-Q26 | `q26_oldest_outbox.sql` | Inspect oldest pending outbox work | `outbox_events_oldest_pending_index` |
| P02-Q27 | `q27_register_trusted_target.sql` | Insert a complete trusted registration once | `targets_external_identity_unique` |
| P02-Q28 | `q28_lock_target_registration.sql` | Lock/read an existing registration after identity conflict | `targets_external_identity_unique` |
| P02-Q29 | `q29_schedule_target_discovery.sql` | Schedule one active discovery intent per target revision | `due_work_target_discovery_active_unique` |

## Lock order reminder (ADR-021)

All multi-row writers acquire present row families in this exact ADR-021 order:
`campaign → campaign-target/target-update → target-enrollment → global/site/rack capacity → submission-admission → operation-reservation → due-work/outbox → audit/decision-input`.

Within a family writers sort by stable UUID or subject key. A writer skips a row
family it does not need; it never reorders remaining families. No query file may
add a `FOR UPDATE` that violates this order.
