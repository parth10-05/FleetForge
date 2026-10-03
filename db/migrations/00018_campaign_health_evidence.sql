-- +goose Up
-- Scheduled health evidence is distinct from a device observation: the
-- schedule proves that a sample was required by the frozen campaign policy at
-- a known logical cut. P03 only persists and evaluates these facts; workers
-- that collect them belong to later execution phases.
CREATE TABLE campaign_health_evidence (
    id uuid PRIMARY KEY,
    campaign_target_id uuid NOT NULL REFERENCES campaign_targets(id) ON DELETE RESTRICT,
    check_name varchar(128) NOT NULL,
    scheduled_at timestamptz NOT NULL,
    observed_at timestamptz NOT NULL,
    passed boolean NOT NULL,
    observation_id uuid REFERENCES device_observations(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaign_health_evidence_target_check_schedule_unique UNIQUE (campaign_target_id, check_name, scheduled_at),
    CONSTRAINT campaign_health_evidence_check_nonempty CHECK (length(check_name) > 0),
    CONSTRAINT campaign_health_evidence_observation_after_schedule CHECK (observed_at >= scheduled_at - interval '366 days')
);
CREATE INDEX campaign_health_evidence_campaign_target_cut_index ON campaign_health_evidence(campaign_target_id, check_name, scheduled_at DESC, observed_at DESC);
COMMENT ON INDEX campaign_health_evidence_campaign_target_cut_index IS 'P03: load scheduled health facts for a frozen cohort at its logical evaluation cut.';

-- +goose Down
DROP INDEX campaign_health_evidence_campaign_target_cut_index;
DROP TABLE campaign_health_evidence;
