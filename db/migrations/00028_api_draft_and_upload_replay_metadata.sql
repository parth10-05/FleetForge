-- +goose Up
-- API-created campaigns retain their exact immutable DRAFT inputs separately
-- from the later validated plan snapshot. This keeps a replayable creation
-- record without allowing plan construction to overwrite the original intent.
CREATE TABLE campaign_drafts (
    campaign_id uuid PRIMARY KEY REFERENCES campaigns(id) ON DELETE RESTRICT,
    artifact_id uuid NOT NULL REFERENCES firmware_artifacts(id) ON DELETE RESTRICT,
    canonical_selector jsonb NOT NULL,
    policy_json jsonb NOT NULL,
    selection_seed varchar(128) NOT NULL,
    selection_algorithm varchar(128) NOT NULL,
    created_by varchar(256) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaign_drafts_selector_object CHECK (jsonb_typeof(canonical_selector) = 'object' AND octet_length(canonical_selector::text) <= 32768),
    CONSTRAINT campaign_drafts_policy_object CHECK (jsonb_typeof(policy_json) = 'object' AND octet_length(policy_json::text) <= 32768),
    CONSTRAINT campaign_drafts_metadata_nonempty CHECK (length(selection_seed) > 0 AND length(selection_algorithm) > 0 AND length(created_by) > 0)
);

-- The artifact subsystem already retains a durable grant identity. The expiry
-- is added independently so older grants remain readable, while API-created
-- grants can retain all non-secret inputs required to recreate a response.
ALTER TABLE firmware_artifacts
    ADD COLUMN upload_grant_expires_at timestamptz;

ALTER TABLE firmware_artifacts
    ADD CONSTRAINT firmware_artifacts_upload_grant_expiry_requires_id
    CHECK (upload_grant_expires_at IS NULL OR upload_grant_id IS NOT NULL);

-- +goose StatementBegin
CREATE FUNCTION reject_campaign_draft_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'campaign drafts are immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER campaign_drafts_append_only
    BEFORE UPDATE OR DELETE ON campaign_drafts
    FOR EACH ROW EXECUTE FUNCTION reject_campaign_draft_mutation();

-- Existing workers may supply legacy grant data incrementally. Once either
-- value has been established it is immutable, preventing later capability
-- reconstruction from silently changing its durable inputs.
-- +goose StatementBegin
CREATE FUNCTION reject_upload_grant_metadata_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.upload_grant_id IS NOT NULL AND NEW.upload_grant_id IS DISTINCT FROM OLD.upload_grant_id)
       OR (OLD.upload_grant_expires_at IS NOT NULL AND NEW.upload_grant_expires_at IS DISTINCT FROM OLD.upload_grant_expires_at) THEN
        RAISE EXCEPTION 'upload grant metadata is immutable' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER firmware_artifacts_upload_grant_metadata_immutable
    BEFORE UPDATE ON firmware_artifacts
    FOR EACH ROW EXECUTE FUNCTION reject_upload_grant_metadata_rewrite();

-- +goose Down
SELECT 1;
