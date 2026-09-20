# Phase 03 connection budget and query-plan method

## Conservative configuration budget

| Variable | Value | Rationale |
|---|---:|---|
| API maximum instances | 4 | Explicit future ECS deployment cap for the Phase 03 calculation |
| API pool maximum | 4 | `FLEETFORGE_POSTGRES_MAX_CONNS` default |
| Worker maximum instances | 8 | Explicit future ECS deployment cap for the Phase 03 calculation |
| Worker pool maximum | 4 | Same bounded application setting |
| Migration connections | 1 | Direct one-session migration path |
| Diagnostic connections | 2 | Reserved read-only operator headroom |
| Operational headroom | 8 | Maintenance, monitoring, and connection replacement allowance |

`(4 × 4) + (8 × 4) + 1 + 2 + 8 = 59` maximum PostgreSQL connections.

This is a configuration calculation, not proof of ECS sizing, Neon limits,
pooler behavior, or 1,000-target throughput. Cloud bootstrap must validate the
actual provider connection limit using separate pooled/direct smoke evidence.

## Reproducible representative plan capture

Use a generated isolated test schema with 1,000 synthetic target-update rows
and run the following commands under the CI PostgreSQL service. Record the
resulting JSON/text plan as CI evidence rather than treating a local laptop
plan as production evidence.

1. Due claim: `EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM due_work WHERE delivery_state='PENDING' AND not_before <= now() ORDER BY not_before, created_at, id LIMIT 32 FOR UPDATE SKIP LOCKED;`
2. Active campaign list: `EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM campaigns WHERE state IN ('RUNNING_CANARY','RUNNING_WAVES') ORDER BY updated_at DESC LIMIT 50;`
3. Active target lookup: `EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM target_updates WHERE campaign_target_id=$1;`
4. Audit timeline: `EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM audit_events WHERE aggregate_type='TARGET_UPDATE' AND aggregate_id=$1 ORDER BY aggregate_revision,event_sequence;`

The due-work, campaign/target, and audit indexes are documented beside their
named query definitions and migration comments. Any measured sequential scan
on the 1,000-target due-work hot path requires a separately reviewed index or
query decision; no claim is made until that measurement is captured in CI.
