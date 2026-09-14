-- +goose Up
CREATE TABLE targets (
    id uuid PRIMARY KEY,
    external_identity varchar(256) NOT NULL,
    endpoint_reference varchar(512) NOT NULL,
    site_key varchar(128) NOT NULL,
    rack_key varchar(128) NOT NULL,
    administrative_state text NOT NULL,
    revision bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT targets_external_identity_unique UNIQUE (external_identity),
    CONSTRAINT targets_administrative_state_known CHECK (administrative_state IN ('ACTIVE', 'DISABLED', 'UNREACHABLE', 'MAINTENANCE_BLOCKED')),
    CONSTRAINT targets_revision_nonnegative CHECK (revision >= 0),
    CONSTRAINT targets_site_key_nonempty CHECK (length(site_key) > 0),
    CONSTRAINT targets_rack_key_nonempty CHECK (length(rack_key) > 0)
);

CREATE TABLE firmware_artifacts (
    id uuid PRIMARY KEY,
    firmware_version varchar(256) NOT NULL,
    sha256_digest varchar(71) NOT NULL,
    object_reference varchar(512) NOT NULL,
    state text NOT NULL,
    revision bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT firmware_artifacts_sha256_canonical CHECK (sha256_digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT firmware_artifacts_state_known CHECK (state IN ('INITIATED', 'UPLOADING', 'UPLOADED', 'VERIFYING', 'VERIFIED', 'REJECTED', 'RETIRED', 'EXPIRED')),
    CONSTRAINT firmware_artifacts_revision_nonnegative CHECK (revision >= 0)
);

CREATE TABLE campaigns (
    id uuid PRIMARY KEY,
    artifact_id uuid NOT NULL REFERENCES firmware_artifacts(id) ON DELETE RESTRICT,
    state text NOT NULL,
    revision bigint NOT NULL DEFAULT 0,
    command_epoch bigint NOT NULL DEFAULT 0,
    suspended_phase text,
    policy_version_id uuid NOT NULL,
    frozen_policy_hash varchar(128),
    frozen_target_snapshot_hash varchar(128),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaigns_state_known CHECK (state IN ('DRAFT', 'VALIDATING', 'READY', 'RUNNING_CANARY', 'EVALUATING_CANARY', 'RUNNING_WAVES', 'PAUSED', 'ABORTING', 'COMPLETED', 'FAILED', 'CANCELLED')),
    CONSTRAINT campaigns_revision_nonnegative CHECK (revision >= 0),
    CONSTRAINT campaigns_command_epoch_nonnegative CHECK (command_epoch >= 0),
    CONSTRAINT campaigns_suspended_phase_valid CHECK (suspended_phase IS NULL OR suspended_phase IN ('RUNNING_CANARY', 'EVALUATING_CANARY', 'RUNNING_WAVES')),
    CONSTRAINT campaigns_ready_snapshot_present CHECK (state <> 'READY' OR (frozen_policy_hash IS NOT NULL AND frozen_target_snapshot_hash IS NOT NULL))
);

CREATE TABLE campaign_waves (
    id uuid PRIMARY KEY,
    campaign_id uuid NOT NULL REFERENCES campaigns(id) ON DELETE RESTRICT,
    ordinal bigint NOT NULL,
    evaluation text NOT NULL DEFAULT 'PENDING',
    released boolean NOT NULL DEFAULT false,
    terminal boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaign_waves_campaign_ordinal_unique UNIQUE (campaign_id, ordinal),
    CONSTRAINT campaign_waves_ordinal_positive CHECK (ordinal > 0),
    CONSTRAINT campaign_waves_evaluation_known CHECK (evaluation IN ('PENDING', 'PASSED', 'FAILED')),
    CONSTRAINT campaign_waves_terminal_evaluation_consistent CHECK (NOT terminal OR evaluation <> 'PENDING')
);

CREATE TABLE campaign_targets (
    id uuid PRIMARY KEY,
    campaign_id uuid NOT NULL REFERENCES campaigns(id) ON DELETE RESTRICT,
    target_id uuid NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    artifact_id uuid NOT NULL REFERENCES firmware_artifacts(id) ON DELETE RESTRICT,
    frozen_target_revision bigint NOT NULL,
    frozen_policy_hash varchar(128) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaign_targets_campaign_target_unique UNIQUE (campaign_id, target_id),
    CONSTRAINT campaign_targets_frozen_revision_nonnegative CHECK (frozen_target_revision >= 0)
);

CREATE TABLE target_updates (
    id uuid PRIMARY KEY,
    campaign_target_id uuid NOT NULL REFERENCES campaign_targets(id) ON DELETE RESTRICT,
    state text NOT NULL,
    revision bigint NOT NULL DEFAULT 0,
    command_epoch bigint NOT NULL DEFAULT 0,
    resume_step text,
    expected_firmware_version varchar(256) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT target_updates_campaign_target_unique UNIQUE (campaign_target_id),
    CONSTRAINT target_updates_state_known CHECK (state IN ('PENDING', 'PRECHECK', 'SUBMITTING', 'UPDATE_ACCEPTED', 'WAITING_REBOOT', 'VERIFYING', 'RETRY_WAIT', 'NEEDS_ATTENTION', 'SUCCEEDED', 'FAILED_TERMINAL', 'SKIPPED', 'CANCELLED')),
    CONSTRAINT target_updates_revision_nonnegative CHECK (revision >= 0),
    CONSTRAINT target_updates_command_epoch_nonnegative CHECK (command_epoch >= 0),
    CONSTRAINT target_updates_resume_step_valid CHECK (resume_step IS NULL OR resume_step IN ('PRECHECK', 'SUBMITTING', 'WAITING_REBOOT', 'VERIFYING')),
    CONSTRAINT target_updates_resume_state_consistent CHECK ((state = 'RETRY_WAIT' AND resume_step IS NOT NULL) OR (state <> 'RETRY_WAIT' AND resume_step IS NULL))
);

CREATE INDEX campaign_waves_campaign_ordinal_index ON campaign_waves (campaign_id, ordinal);
COMMENT ON INDEX campaign_waves_campaign_ordinal_index IS 'P03-Q01: load campaign waves in release and evaluation order.';
CREATE INDEX campaign_targets_target_index ON campaign_targets (target_id, campaign_id);
COMMENT ON INDEX campaign_targets_target_index IS 'P03-Q02: find a target campaign membership before enrollment and readiness decisions.';
CREATE INDEX target_updates_campaign_target_index ON target_updates (campaign_target_id);
COMMENT ON INDEX target_updates_campaign_target_index IS 'P03-Q03: load a target update by frozen campaign target identity.';

-- +goose Down
SELECT 1;
