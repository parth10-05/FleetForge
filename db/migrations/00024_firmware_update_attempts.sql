-- +goose Up
-- P05 separates a target-facing attempt from generic workflow bookkeeping.
-- Attempts are permanent evidence; retry creates a child only after an
-- independently proven absence, never by replaying an ambiguous request.
CREATE TABLE firmware_update_attempts (
    id uuid PRIMARY KEY,
    step_attempt_id uuid NOT NULL UNIQUE REFERENCES step_attempts(id) ON DELETE RESTRICT,
    submission_admission_id uuid NOT NULL REFERENCES submission_admissions(id) ON DELETE RESTRICT,
    parent_attempt_id uuid REFERENCES firmware_update_attempts(id) ON DELETE RESTRICT,
    target_update_id uuid NOT NULL REFERENCES target_updates(id) ON DELETE RESTRICT,
    campaign_id uuid NOT NULL REFERENCES campaigns(id) ON DELETE RESTRICT,
    target_id uuid NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    artifact_id uuid NOT NULL REFERENCES firmware_artifacts(id) ON DELETE RESTRICT,
    provider varchar(32) NOT NULL,
    container varchar(256) NOT NULL,
    object_key varchar(256) NOT NULL,
    object_version varchar(256) NOT NULL,
    object_size bigint NOT NULL,
    sha256_digest varchar(71) NOT NULL,
    expected_version varchar(256) NOT NULL,
    update_mechanism varchar(32) NOT NULL,
    request_fingerprint varchar(71) NOT NULL,
    captured_command_epoch bigint NOT NULL,
    admitted_revision bigint NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'ADMITTED',
    result_class varchar(64),
    remote_task_reference varchar(512),
    response_fingerprint varchar(71),
    started_at timestamptz NOT NULL DEFAULT now(),
    transport_finished_at timestamptz,
    accepted_at timestamptz,
    resolved_at timestamptz,
    CONSTRAINT firmware_update_attempts_identity CHECK (
        length(provider)>0 AND length(container)>0 AND length(object_key)>0 AND length(object_version)>0
        AND object_key !~ '(^|/)\.\.(/|$)' AND object_key !~ '^/'
        AND object_size > 0 AND object_size <= 16777216
        AND sha256_digest ~ '^sha256:[0-9a-f]{64}$'
        AND expected_version <> '' AND request_fingerprint ~ '^sha256:[0-9a-f]{64}$'
        AND update_mechanism = 'HTTP_PUSH_V1'
    ),
    CONSTRAINT firmware_update_attempts_fences CHECK (captured_command_epoch >= 0 AND admitted_revision >= 0),
    CONSTRAINT firmware_update_attempts_status_known CHECK (status IN ('ADMITTED','ACCEPTED','AMBIGUOUS','REJECTED','PROVEN_ABSENT','ADOPTED','SUCCEEDED','FAILED_TERMINAL','NEEDS_ATTENTION')),
    CONSTRAINT firmware_update_attempts_task_reference_safe CHECK (remote_task_reference IS NULL OR (remote_task_reference ~ '^/redfish/v1/' AND length(remote_task_reference) <= 512)),
    CONSTRAINT firmware_update_attempts_result_fingerprint CHECK (response_fingerprint IS NULL OR response_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT firmware_update_attempts_timestamps CHECK (
        (transport_finished_at IS NULL OR transport_finished_at >= started_at)
        AND (accepted_at IS NULL OR (transport_finished_at IS NOT NULL AND accepted_at >= started_at))
        AND ((status IN ('ADMITTED','ACCEPTED','AMBIGUOUS') AND resolved_at IS NULL) OR (status IN ('REJECTED','PROVEN_ABSENT','ADOPTED','SUCCEEDED','FAILED_TERMINAL','NEEDS_ATTENTION') AND resolved_at IS NOT NULL))
    ),
    CONSTRAINT firmware_update_attempts_child_requires_parent CHECK (parent_attempt_id IS NULL OR parent_attempt_id <> id)
);

CREATE UNIQUE INDEX firmware_update_attempts_initial_admission_unique
    ON firmware_update_attempts (submission_admission_id) WHERE parent_attempt_id IS NULL;
CREATE UNIQUE INDEX firmware_update_attempts_active_child_unique
    ON firmware_update_attempts (submission_admission_id)
    WHERE parent_attempt_id IS NOT NULL AND status IN ('ADMITTED','ACCEPTED','AMBIGUOUS');
CREATE INDEX firmware_update_attempts_target_timeline_index ON firmware_update_attempts (target_update_id, started_at DESC);
CREATE INDEX firmware_update_attempts_admission_timeline_index ON firmware_update_attempts (submission_admission_id, started_at DESC);
CREATE INDEX firmware_update_attempts_request_fingerprint_index ON firmware_update_attempts (request_fingerprint);

ALTER TABLE submission_admissions
    ADD COLUMN initial_attempt_id uuid,
    ADD COLUMN active_attempt_id uuid,
    ADD COLUMN retry_authorizations bigint NOT NULL DEFAULT 0 CHECK (retry_authorizations >= 0),
    ADD CONSTRAINT submission_admissions_initial_attempt_fk FOREIGN KEY (initial_attempt_id) REFERENCES firmware_update_attempts(id) ON DELETE RESTRICT,
    ADD CONSTRAINT submission_admissions_active_attempt_fk FOREIGN KEY (active_attempt_id) REFERENCES firmware_update_attempts(id) ON DELETE RESTRICT;

CREATE TABLE ambiguity_resolutions (
    id uuid PRIMARY KEY,
    attempt_id uuid NOT NULL REFERENCES firmware_update_attempts(id) ON DELETE RESTRICT,
    target_update_revision bigint NOT NULL CHECK (target_update_revision >= 0),
    outcome varchar(32) NOT NULL CHECK (outcome IN ('PROVEN_ABSENT','ADOPTED','NEEDS_ATTENTION','TERMINAL_FAILURE')),
    evidence_summary jsonb NOT NULL CHECK (jsonb_typeof(evidence_summary)='object' AND octet_length(evidence_summary::text)<=32768),
    evidence_fingerprint varchar(71) NOT NULL CHECK (evidence_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
    journal_run_id varchar(128),
    journal_checkpoint_hash varchar(71),
    journal_sequence_start bigint,
    journal_sequence_end bigint,
    journal_confirmed_absent boolean NOT NULL DEFAULT false,
    late_caller_excluded boolean NOT NULL DEFAULT false,
    late_caller_exclusion_method varchar(128),
    proof_reference varchar(512),
    actor_id varchar(256) NOT NULL,
    command_id uuid NOT NULL,
    reason_code varchar(128) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    supersedes_id uuid REFERENCES ambiguity_resolutions(id) ON DELETE RESTRICT,
    CONSTRAINT ambiguity_resolutions_absence_proof CHECK (outcome <> 'PROVEN_ABSENT' OR (journal_confirmed_absent AND late_caller_excluded AND late_caller_exclusion_method IS NOT NULL AND proof_reference IS NOT NULL)),
    CONSTRAINT ambiguity_resolutions_journal_bounds CHECK ((journal_sequence_start IS NULL AND journal_sequence_end IS NULL) OR (journal_sequence_start >= 0 AND journal_sequence_end >= journal_sequence_start)),
    CONSTRAINT ambiguity_resolutions_checkpoint CHECK (journal_checkpoint_hash IS NULL OR journal_checkpoint_hash ~ '^sha256:[0-9a-f]{64}$')
);

-- +goose StatementBegin
CREATE FUNCTION fleetforge_reject_execution_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'FleetForge execution evidence is append-only';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER ambiguity_resolutions_immutable
    BEFORE UPDATE OR DELETE ON ambiguity_resolutions
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_execution_evidence_mutation();

-- +goose Down
SELECT 1;
