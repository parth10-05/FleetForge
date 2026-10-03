-- P03-Q35: load-campaign-plan-membership.sql
-- Access pattern: load a frozen campaign plan's one canary and one later wave
-- in persisted execution order for readiness, start, evaluation, and replay.
-- name: LoadCampaignPlanMembership :many
SELECT ct.id, ct.target_id, ct.wave_id, ct.cohort, ct.cohort_ordinal,
       ct.selection_reason, ct.frozen_target_revision, w.ordinal AS wave_ordinal,
       w.evaluation, w.released, w.terminal
  FROM campaign_targets ct
  JOIN campaign_waves w ON w.id = ct.wave_id
 WHERE ct.campaign_id = $1
 ORDER BY w.ordinal, ct.cohort_ordinal, ct.target_id;
