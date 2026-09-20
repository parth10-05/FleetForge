-- +goose Up
CREATE TABLE target_enrollments (
    id uuid PRIMARY KEY,
    target_id uuid NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    campaign_id uuid NOT NULL REFERENCES campaigns(id) ON DELETE RESTRICT,
    campaign_target_id uuid NOT NULL REFERENCES campaign_targets(id) ON DELETE RESTRICT,
    state text NOT NULL DEFAULT 'ACTIVE',
    acquired_at timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,
    release_reason varchar(128),
    CONSTRAINT target_enrollments_campaign_target_unique UNIQUE (campaign_target_id),
    CONSTRAINT target_enrollments_state_known CHECK (state IN ('ACTIVE', 'RELEASED')),
    CONSTRAINT target_enrollments_release_consistent CHECK ((state = 'ACTIVE' AND released_at IS NULL AND release_reason IS NULL) OR (state = 'RELEASED' AND released_at IS NOT NULL AND release_reason IS NOT NULL))
);
CREATE UNIQUE INDEX target_enrollments_one_active_target ON target_enrollments (target_id) WHERE state = 'ACTIVE';
COMMENT ON INDEX target_enrollments_one_active_target IS 'P03-Q04: readiness ownership gate prevents a target joining two active campaigns.';

CREATE TABLE submission_admissions (
    id uuid PRIMARY KEY,
    campaign_id uuid NOT NULL REFERENCES campaigns(id) ON DELETE RESTRICT,
    target_id uuid NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    artifact_id uuid NOT NULL REFERENCES firmware_artifacts(id) ON DELETE RESTRICT,
    target_update_id uuid NOT NULL REFERENCES target_updates(id) ON DELETE RESTRICT,
    command_id uuid NOT NULL,
    step varchar(64) NOT NULL,
    captured_command_epoch bigint NOT NULL,
    state text NOT NULL DEFAULT 'ADMITTED',
    admitted_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    resolution_reason varchar(128),
    CONSTRAINT submission_admissions_identity_unique UNIQUE (target_id, campaign_id, artifact_id, step),
    CONSTRAINT submission_admissions_command_unique UNIQUE (command_id),
    CONSTRAINT submission_admissions_epoch_nonnegative CHECK (captured_command_epoch >= 0),
    CONSTRAINT submission_admissions_step_nonempty CHECK (length(step) > 0),
    CONSTRAINT submission_admissions_state_known CHECK (state IN ('ADMITTED', 'POSSIBLY_IN_FLIGHT', 'RESOLVED', 'NEEDS_ATTENTION')),
    CONSTRAINT submission_admissions_resolution_consistent CHECK ((state IN ('ADMITTED', 'POSSIBLY_IN_FLIGHT', 'NEEDS_ATTENTION') AND resolved_at IS NULL) OR (state = 'RESOLVED' AND resolved_at IS NOT NULL AND resolution_reason IS NOT NULL))
);

CREATE TABLE operation_reservations (
    id uuid PRIMARY KEY,
    admission_id uuid NOT NULL REFERENCES submission_admissions(id) ON DELETE RESTRICT,
    target_id uuid NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    campaign_id uuid NOT NULL REFERENCES campaigns(id) ON DELETE RESTRICT,
    scope_level text NOT NULL,
    scope_key varchar(128) NOT NULL,
    state text NOT NULL DEFAULT 'ACTIVE',
    reserved_at timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,
    release_reason varchar(128),
    CONSTRAINT operation_reservations_attempt_scope_unique UNIQUE (admission_id, scope_level, scope_key),
    CONSTRAINT operation_reservations_scope_known CHECK (scope_level IN ('GLOBAL', 'SITE', 'RACK')),
    CONSTRAINT operation_reservations_scope_key_nonempty CHECK (length(scope_key) > 0),
    CONSTRAINT operation_reservations_global_key CHECK (scope_level <> 'GLOBAL' OR scope_key = 'GLOBAL'),
    CONSTRAINT operation_reservations_state_known CHECK (state IN ('ACTIVE', 'QUARANTINED', 'RELEASED')),
    CONSTRAINT operation_reservations_release_consistent CHECK ((state IN ('ACTIVE', 'QUARANTINED') AND released_at IS NULL AND release_reason IS NULL) OR (state = 'RELEASED' AND released_at IS NOT NULL AND release_reason IS NOT NULL))
);
CREATE UNIQUE INDEX operation_reservations_one_active_scope ON operation_reservations (admission_id, scope_level, scope_key) WHERE state IN ('ACTIVE', 'QUARANTINED');
COMMENT ON INDEX operation_reservations_one_active_scope IS 'P03-Q05: one durable physical-operation reservation per admission and scope.';
CREATE INDEX operation_reservations_target_active_index ON operation_reservations (target_id, state) WHERE state IN ('ACTIVE', 'QUARANTINED');
COMMENT ON INDEX operation_reservations_target_active_index IS 'P03-Q06: parent release and quarantine checks find unresolved target operations.';

-- +goose Down
SELECT 1;
