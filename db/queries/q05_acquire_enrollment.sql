-- P03-Q05: acquire-enrollment.sql
-- Access pattern: insert an enrollment claim for a frozen target in a readiness transaction.
-- Uniqueness enforced by: target_enrollments_one_active_target partial index.
-- Lock order: campaign row → target row → enrollment insert (ADR-021).
INSERT INTO target_enrollments (id, target_id, campaign_id, campaign_target_id)
VALUES ($1, $2, $3, $4);
