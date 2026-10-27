-- +goose Up
-- API-created upload grants retain the non-secret claim facts needed to
-- recreate a signed direct-upload capability after idempotent replay.
ALTER TABLE firmware_artifacts
    ADD COLUMN upload_size_bytes bigint,
    ADD COLUMN upload_media_type varchar(256);

ALTER TABLE firmware_artifacts
    ADD CONSTRAINT firmware_artifacts_upload_claims_complete
    CHECK (
        (upload_size_bytes IS NULL AND upload_media_type IS NULL)
        OR (
            upload_size_bytes > 0
            AND upload_size_bytes <= 16777216
            AND upload_media_type = 'application/octet-stream'
        )
    );

-- +goose StatementBegin
CREATE FUNCTION reject_upload_claim_metadata_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.upload_size_bytes IS NOT NULL AND NEW.upload_size_bytes IS DISTINCT FROM OLD.upload_size_bytes)
       OR (OLD.upload_media_type IS NOT NULL AND NEW.upload_media_type IS DISTINCT FROM OLD.upload_media_type) THEN
        RAISE EXCEPTION 'upload claim metadata is immutable' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER firmware_artifacts_upload_claim_metadata_immutable
    BEFORE UPDATE ON firmware_artifacts
    FOR EACH ROW EXECUTE FUNCTION reject_upload_claim_metadata_rewrite();

-- +goose Down
SELECT 1;
