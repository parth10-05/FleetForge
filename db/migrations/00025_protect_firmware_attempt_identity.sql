-- +goose Up
-- A submit result may advance its outcome fields, but it must never rewrite
-- the exact request that reached (or may have reached) a target.
CREATE FUNCTION fleetforge_reject_firmware_attempt_identity_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.step_attempt_id IS DISTINCT FROM NEW.step_attempt_id
       OR OLD.submission_admission_id IS DISTINCT FROM NEW.submission_admission_id
       OR OLD.parent_attempt_id IS DISTINCT FROM NEW.parent_attempt_id
       OR OLD.target_update_id IS DISTINCT FROM NEW.target_update_id
       OR OLD.campaign_id IS DISTINCT FROM NEW.campaign_id
       OR OLD.target_id IS DISTINCT FROM NEW.target_id
       OR OLD.artifact_id IS DISTINCT FROM NEW.artifact_id
       OR OLD.provider IS DISTINCT FROM NEW.provider
       OR OLD.container IS DISTINCT FROM NEW.container
       OR OLD.object_key IS DISTINCT FROM NEW.object_key
       OR OLD.object_version IS DISTINCT FROM NEW.object_version
       OR OLD.object_size IS DISTINCT FROM NEW.object_size
       OR OLD.sha256_digest IS DISTINCT FROM NEW.sha256_digest
       OR OLD.expected_version IS DISTINCT FROM NEW.expected_version
       OR OLD.update_mechanism IS DISTINCT FROM NEW.update_mechanism
       OR OLD.request_fingerprint IS DISTINCT FROM NEW.request_fingerprint
       OR OLD.captured_command_epoch IS DISTINCT FROM NEW.captured_command_epoch
       OR OLD.admitted_revision IS DISTINCT FROM NEW.admitted_revision
       OR OLD.started_at IS DISTINCT FROM NEW.started_at THEN
        RAISE EXCEPTION 'FleetForge firmware attempt identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER firmware_update_attempts_identity_immutable
    BEFORE UPDATE OR DELETE ON firmware_update_attempts
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_firmware_attempt_identity_mutation();

-- +goose Down
SELECT 1;
