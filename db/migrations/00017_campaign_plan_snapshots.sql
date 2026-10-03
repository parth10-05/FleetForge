-- +goose Up
-- Immutable V1 campaign-plan evidence. Campaign membership remains authoritative
-- in campaign_targets; this table retains the canonical policy and plan that
-- explain that membership and its exact P02 candidate/artifact inputs.
CREATE TABLE campaign_plan_snapshots (
    campaign_id uuid PRIMARY KEY REFERENCES campaigns(id) ON DELETE RESTRICT,
    candidate_id uuid NOT NULL REFERENCES preflight_candidates(id) ON DELETE RESTRICT,
    draft_revision bigint NOT NULL,
    policy_json jsonb NOT NULL,
    effective_plan_json jsonb NOT NULL,
    seed varchar(128) NOT NULL,
    policy_version varchar(128) NOT NULL,
    algorithm_version varchar(128) NOT NULL,
    plan_version varchar(128) NOT NULL,
    artifact_version_id varchar(256) NOT NULL,
    artifact_digest varchar(71) NOT NULL,
    verification_policy_version varchar(64) NOT NULL,
    target_snapshot_hash varchar(71) NOT NULL,
    plan_hash varchar(71) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaign_plan_snapshot_revision_nonnegative CHECK (draft_revision >= 0),
    CONSTRAINT campaign_plan_snapshot_json_bounded CHECK (jsonb_typeof(policy_json)='object' AND jsonb_typeof(effective_plan_json)='object' AND octet_length(policy_json::text) <= 32768 AND octet_length(effective_plan_json::text) <= 32768),
    CONSTRAINT campaign_plan_snapshot_values CHECK (length(seed)>0 AND length(policy_version)>0 AND length(algorithm_version)>0 AND length(plan_version)>0 AND artifact_version_id <> '' AND artifact_digest ~ '^sha256:[0-9a-f]{64}$' AND verification_policy_version <> '' AND target_snapshot_hash ~ '^sha256:[0-9a-f]{64}$' AND plan_hash ~ '^sha256:[0-9a-f]{64}$')
);

ALTER TABLE campaign_waves ADD COLUMN cohort varchar(32) NOT NULL DEFAULT 'CANARY';
ALTER TABLE campaign_waves ADD CONSTRAINT campaign_waves_cohort_known CHECK (cohort IN ('CANARY','LATER_WAVE'));
ALTER TABLE campaign_waves ADD CONSTRAINT campaign_waves_cohort_unique UNIQUE (campaign_id, cohort);

ALTER TABLE campaign_targets ADD COLUMN wave_id uuid REFERENCES campaign_waves(id) ON DELETE RESTRICT;
ALTER TABLE campaign_targets ADD COLUMN cohort varchar(32) NOT NULL DEFAULT 'CANARY';
ALTER TABLE campaign_targets ADD COLUMN cohort_ordinal bigint NOT NULL DEFAULT 1;
ALTER TABLE campaign_targets ADD COLUMN selection_reason varchar(128) NOT NULL DEFAULT 'legacy.unplanned';
ALTER TABLE campaign_targets ADD CONSTRAINT campaign_targets_cohort_known CHECK (cohort IN ('CANARY','LATER_WAVE'));
ALTER TABLE campaign_targets ADD CONSTRAINT campaign_targets_cohort_ordinal_positive CHECK (cohort_ordinal > 0);
CREATE UNIQUE INDEX campaign_targets_campaign_wave_ordinal_unique ON campaign_targets(campaign_id,wave_id,cohort_ordinal) WHERE wave_id IS NOT NULL;
CREATE INDEX campaign_targets_campaign_wave_index ON campaign_targets(campaign_id,wave_id,cohort_ordinal);
COMMENT ON INDEX campaign_targets_campaign_wave_index IS 'P03-Q35: load immutable campaign cohort membership in execution order.';

-- Frozen plan and assignments cannot be rewritten after validation. A new
-- draft revision must use a new campaign identity in V1.
CREATE TRIGGER campaign_plan_snapshots_append_only
    BEFORE UPDATE OR DELETE ON campaign_plan_snapshots
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_immutable_evidence_mutation();

-- +goose Down
DROP TRIGGER campaign_plan_snapshots_append_only ON campaign_plan_snapshots;
DROP INDEX campaign_targets_campaign_wave_index;
DROP INDEX campaign_targets_campaign_wave_ordinal_unique;
ALTER TABLE campaign_targets DROP CONSTRAINT campaign_targets_cohort_ordinal_positive;
ALTER TABLE campaign_targets DROP CONSTRAINT campaign_targets_cohort_known;
ALTER TABLE campaign_targets DROP COLUMN selection_reason;
ALTER TABLE campaign_targets DROP COLUMN cohort_ordinal;
ALTER TABLE campaign_targets DROP COLUMN cohort;
ALTER TABLE campaign_targets DROP COLUMN wave_id;
ALTER TABLE campaign_waves DROP CONSTRAINT campaign_waves_cohort_unique;
ALTER TABLE campaign_waves DROP CONSTRAINT campaign_waves_cohort_known;
ALTER TABLE campaign_waves DROP COLUMN cohort;
DROP TABLE campaign_plan_snapshots;
