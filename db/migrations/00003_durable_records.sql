-- +goose Up
-- Durable records deliberately use text discriminators plus constraints. New
-- vocabulary can be rolled out without an enum-alter deployment, while values
-- outside the Phase 03 contract fail at the database boundary.

CREATE TABLE step_attempts (
    id uuid PRIMARY KEY,
    target_update_id uuid NOT NULL REFERENCES target_updates(id) ON DELETE RESTRICT,
    step varchar(64) NOT NULL,
    attempt_number bigint NOT NULL,
    command_id uuid NOT NULL,
    outcome text NOT NULL DEFAULT 'INTENDED',
    evidence_hash varchar(128),
    remote_task_reference varchar(512),
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    CONSTRAINT step_attempts_update_step_attempt_unique UNIQUE (target_update_id, step, attempt_number),
    CONSTRAINT step_attempts_command_unique UNIQUE (command_id),
    CONSTRAINT step_attempts_step_nonempty CHECK (length(step) > 0),
    CONSTRAINT step_attempts_attempt_positive CHECK (attempt_number > 0),
    CONSTRAINT step_attempts_outcome_known CHECK (outcome IN ('INTENDED', 'SUCCEEDED', 'RETRYABLE_FAILURE', 'TERMINAL_FAILURE', 'AMBIGUOUS')),
    CONSTRAINT step_attempts_completion_consistent CHECK ((outcome = 'INTENDED' AND completed_at IS NULL) OR (outcome <> 'INTENDED' AND completed_at IS NOT NULL))
);

CREATE TABLE device_observations (
    id uuid PRIMARY KEY,
    target_id uuid NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    target_update_id uuid REFERENCES target_updates(id) ON DELETE RESTRICT,
    kind varchar(64) NOT NULL,
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    normalized_data jsonb NOT NULL,
    raw_object_reference varchar(512),
    CONSTRAINT device_observations_kind_nonempty CHECK (length(kind) > 0),
    CONSTRAINT device_observations_data_object CHECK (jsonb_typeof(normalized_data) = 'object'),
    CONSTRAINT device_observations_data_bounded CHECK (octet_length(normalized_data::text) <= 32768),
    CONSTRAINT device_observations_time_order CHECK (received_at >= observed_at - interval '366 days')
);

CREATE TABLE idempotency_records (
    id uuid PRIMARY KEY,
    actor_id varchar(256) NOT NULL,
    route varchar(256) NOT NULL,
    key_hash varchar(128) NOT NULL,
    request_hash varchar(128) NOT NULL,
    command_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'IN_PROGRESS',
    owner_token uuid NOT NULL,
    claim_expires_at timestamptz NOT NULL,
    completed_at timestamptz,
    response_status integer,
    response_reference varchar(512),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT idempotency_records_actor_route_key_unique UNIQUE (actor_id, route, key_hash),
    CONSTRAINT idempotency_records_actor_nonempty CHECK (length(actor_id) > 0),
    CONSTRAINT idempotency_records_route_canonical CHECK (route ~ '^/[A-Za-z0-9._~!$&''()*+,;=:@/%-]*$'),
    CONSTRAINT idempotency_records_hashes_nonempty CHECK (length(key_hash) >= 32 AND length(request_hash) >= 32),
    CONSTRAINT idempotency_records_state_known CHECK (state IN ('IN_PROGRESS', 'COMPLETED', 'RECOVERY_REQUIRED')),
    CONSTRAINT idempotency_records_completion_consistent CHECK ((state = 'COMPLETED' AND completed_at IS NOT NULL AND response_status BETWEEN 100 AND 599 AND response_reference IS NOT NULL) OR (state <> 'COMPLETED' AND completed_at IS NULL AND response_status IS NULL AND response_reference IS NULL)),
    CONSTRAINT idempotency_records_expiry_future CHECK (claim_expires_at > created_at)
);

CREATE TABLE decision_inputs (
    id uuid PRIMARY KEY,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_revision bigint NOT NULL,
    decision_sequence bigint NOT NULL,
    logical_time timestamptz NOT NULL,
    reducer_version varchar(64) NOT NULL,
    policy_version varchar(128) NOT NULL,
    normalized_facts jsonb NOT NULL,
    result text NOT NULL,
    reason_code varchar(128) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT decision_inputs_aggregate_revision_sequence_unique UNIQUE (aggregate_type, aggregate_id, aggregate_revision, decision_sequence),
    CONSTRAINT decision_inputs_aggregate_known CHECK (aggregate_type IN ('CAMPAIGN', 'TARGET_UPDATE')),
    CONSTRAINT decision_inputs_revision_nonnegative CHECK (aggregate_revision >= 0 AND decision_sequence >= 0),
    CONSTRAINT decision_inputs_versions_nonempty CHECK (length(reducer_version) > 0 AND length(policy_version) > 0),
    CONSTRAINT decision_inputs_facts_object CHECK (jsonb_typeof(normalized_facts) = 'object'),
    CONSTRAINT decision_inputs_facts_bounded CHECK (octet_length(normalized_facts::text) <= 32768),
    CONSTRAINT decision_inputs_result_nonempty CHECK (length(result) > 0 AND length(reason_code) > 0)
);

CREATE TABLE audit_events (
    id bigserial PRIMARY KEY,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_revision bigint NOT NULL,
    event_sequence bigint NOT NULL,
    actor_id varchar(256) NOT NULL,
    command_id uuid,
    action varchar(128) NOT NULL,
    reason_code varchar(128) NOT NULL,
    before_hash varchar(128),
    after_hash varchar(128),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT audit_events_aggregate_revision_sequence_unique UNIQUE (aggregate_type, aggregate_id, aggregate_revision, event_sequence),
    CONSTRAINT audit_events_aggregate_known CHECK (aggregate_type IN ('CAMPAIGN', 'TARGET_UPDATE')),
    CONSTRAINT audit_events_revision_nonnegative CHECK (aggregate_revision >= 0 AND event_sequence >= 0),
    CONSTRAINT audit_events_required_fields_nonempty CHECK (length(actor_id) > 0 AND length(action) > 0 AND length(reason_code) > 0)
);

-- +goose StatementBegin
CREATE FUNCTION reject_audit_event_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events are append-only' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER audit_events_append_only BEFORE UPDATE OR DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();

CREATE TABLE outbox_events (
    event_id uuid PRIMARY KEY,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_revision bigint NOT NULL,
    event_type varchar(128) NOT NULL,
    schema_version integer NOT NULL,
    payload jsonb NOT NULL,
    delivery_state text NOT NULL DEFAULT 'PENDING',
    delivery_attempts bigint NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error_class varchar(128),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT outbox_events_aggregate_known CHECK (aggregate_type IN ('CAMPAIGN', 'TARGET_UPDATE')),
    CONSTRAINT outbox_events_revision_nonnegative CHECK (aggregate_revision >= 0),
    CONSTRAINT outbox_events_type_nonempty CHECK (length(event_type) > 0),
    CONSTRAINT outbox_events_schema_version_positive CHECK (schema_version > 0),
    CONSTRAINT outbox_events_payload_object CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT outbox_events_payload_bounded CHECK (octet_length(payload::text) <= 32768),
    CONSTRAINT outbox_events_attempts_nonnegative CHECK (delivery_attempts >= 0),
    CONSTRAINT outbox_events_state_known CHECK (delivery_state IN ('PENDING', 'CLAIMED', 'PUBLISHED', 'QUARANTINED')),
    CONSTRAINT outbox_events_publication_consistent CHECK ((delivery_state = 'PUBLISHED' AND published_at IS NOT NULL) OR (delivery_state <> 'PUBLISHED' AND published_at IS NULL))
);

CREATE TABLE due_work (
    id uuid PRIMARY KEY,
    command_id uuid NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    work_type varchar(128) NOT NULL,
    schema_version integer NOT NULL,
    not_before timestamptz NOT NULL,
    expected_revision bigint NOT NULL,
    expected_command_epoch bigint,
    delivery_state text NOT NULL DEFAULT 'PENDING',
    delivery_generation bigint NOT NULL DEFAULT 0,
    lease_token uuid,
    lease_owner varchar(256),
    lease_expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT due_work_command_generation_unique UNIQUE (command_id, delivery_generation),
    CONSTRAINT due_work_semantic_unique UNIQUE (subject_type, subject_id, work_type, command_id),
    CONSTRAINT due_work_subject_known CHECK (subject_type IN ('TARGET_UPDATE', 'CAMPAIGN', 'ARTIFACT', 'VALIDATION')),
    CONSTRAINT due_work_type_nonempty CHECK (length(work_type) > 0),
    CONSTRAINT due_work_schema_version_positive CHECK (schema_version > 0),
    CONSTRAINT due_work_revisions_nonnegative CHECK (expected_revision >= 0 AND (expected_command_epoch IS NULL OR expected_command_epoch >= 0)),
    CONSTRAINT due_work_generation_nonnegative CHECK (delivery_generation >= 0),
    CONSTRAINT due_work_state_known CHECK (delivery_state IN ('PENDING', 'CLAIMED', 'COMPLETED', 'RECONCILIATION_REQUIRED', 'CANCELLED')),
    CONSTRAINT due_work_lease_consistent CHECK ((delivery_state = 'CLAIMED' AND lease_token IS NOT NULL AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL) OR (delivery_state <> 'CLAIMED' AND lease_token IS NULL AND lease_owner IS NULL AND lease_expires_at IS NULL))
);

CREATE INDEX device_observations_target_kind_time_index ON device_observations (target_id, kind, observed_at DESC);
COMMENT ON INDEX device_observations_target_kind_time_index IS 'P03-Q07: inspect the latest normalized evidence for a target and observation kind.';
CREATE INDEX device_observations_update_time_index ON device_observations (target_update_id, observed_at DESC) WHERE target_update_id IS NOT NULL;
COMMENT ON INDEX device_observations_update_time_index IS 'P03-Q08: reconstruct a target-update evidence timeline.';
CREATE INDEX outbox_events_publishable_index ON outbox_events (next_attempt_at, created_at) WHERE delivery_state = 'PENDING';
COMMENT ON INDEX outbox_events_publishable_index IS 'P03-Q09: ordered SKIP LOCKED publisher claims for unpublished events.';
CREATE INDEX due_work_claimable_index ON due_work (not_before, created_at) WHERE delivery_state = 'PENDING';
COMMENT ON INDEX due_work_claimable_index IS 'P03-Q10: ordered SKIP LOCKED claims for due work.';
CREATE INDEX audit_events_timeline_index ON audit_events (aggregate_type, aggregate_id, aggregate_revision, event_sequence);
COMMENT ON INDEX audit_events_timeline_index IS 'P03-Q11: reconstruct an immutable aggregate audit timeline.';

-- +goose Down
SELECT 1;
