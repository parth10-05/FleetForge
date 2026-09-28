-- +goose Up
-- P02 evidence is append-only. Administrative lifecycle transitions live on
-- firmware_artifacts; they never rewrite the object version or frozen snapshot.
CREATE FUNCTION fleetforge_reject_immutable_evidence_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'immutable FleetForge evidence cannot be modified or deleted'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER artifact_object_versions_immutable
    BEFORE UPDATE OR DELETE ON artifact_object_versions
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_immutable_evidence_mutation();
CREATE TRIGGER artifact_verification_results_immutable
    BEFORE UPDATE OR DELETE ON artifact_verification_results
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_immutable_evidence_mutation();
CREATE TRIGGER preflight_candidates_immutable
    BEFORE UPDATE OR DELETE ON preflight_candidates
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_immutable_evidence_mutation();
CREATE TRIGGER preflight_candidate_targets_immutable
    BEFORE UPDATE OR DELETE ON preflight_candidate_targets
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_immutable_evidence_mutation();
CREATE TRIGGER artifact_holds_immutable
    BEFORE DELETE ON artifact_holds
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_immutable_evidence_mutation();
-- Holds may only be released once; changing their identity is forbidden.
CREATE FUNCTION fleetforge_guard_artifact_hold_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'immutable FleetForge artifact hold cannot be deleted'
            USING ERRCODE = '55000';
    END IF;
    IF OLD.artifact_id <> NEW.artifact_id OR OLD.campaign_id <> NEW.campaign_id
       OR OLD.object_version_id <> NEW.object_version_id OR OLD.acquired_at <> NEW.acquired_at
       OR OLD.released_at IS NOT NULL THEN
        RAISE EXCEPTION 'immutable FleetForge artifact hold cannot be rewritten'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER artifact_holds_immutable ON artifact_holds;
CREATE TRIGGER artifact_holds_release_once
    BEFORE UPDATE OR DELETE ON artifact_holds
    FOR EACH ROW EXECUTE FUNCTION fleetforge_guard_artifact_hold_update();
-- +goose Down
DROP TRIGGER artifact_holds_release_once ON artifact_holds;
DROP TRIGGER preflight_candidate_targets_immutable ON preflight_candidate_targets;
DROP TRIGGER preflight_candidates_immutable ON preflight_candidates;
DROP TRIGGER artifact_verification_results_immutable ON artifact_verification_results;
DROP TRIGGER artifact_object_versions_immutable ON artifact_object_versions;
DROP FUNCTION fleetforge_guard_artifact_hold_update();
DROP FUNCTION fleetforge_reject_immutable_evidence_mutation();
