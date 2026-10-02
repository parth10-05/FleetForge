-- +goose Up
-- Cleanup may act only on a grant identity durably tied to an expired upload.
-- This is intentionally separate from immutable bound object versions.
ALTER TABLE firmware_artifacts
    ADD COLUMN upload_grant_id varchar(128);

ALTER TABLE firmware_artifacts
    ADD CONSTRAINT firmware_artifacts_upload_grant_id_valid
    CHECK (upload_grant_id IS NULL OR upload_grant_id ~ '^[A-Za-z0-9._-]{1,128}$');

CREATE UNIQUE INDEX firmware_artifacts_upload_grant_id_unique
    ON firmware_artifacts(upload_grant_id)
    WHERE upload_grant_id IS NOT NULL;

-- +goose Down
SELECT 1;
