-- P03-Q02: find-target-campaign-membership.sql
-- Access pattern: find a target's campaign memberships before enrollment and readiness decisions.
-- Index: campaign_targets_target_index ON campaign_targets (target_id, campaign_id)
SELECT ct.id,
       ct.campaign_id,
       ct.target_id,
       ct.artifact_id,
       ct.frozen_target_revision,
       ct.frozen_policy_hash,
       ct.created_at
  FROM campaign_targets ct
 WHERE ct.target_id = $1;
