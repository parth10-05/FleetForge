-- +goose Up
-- The singleton is deliberately diagnostic-only. It contains only bounded
-- numeric gauges and never retains workflow/provider identity.
CREATE TABLE wakeup_telemetry_snapshot (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    observed_at timestamptz NOT NULL,
    pending_wakeup_count bigint NOT NULL CHECK (pending_wakeup_count >= 0),
    oldest_wakeup_age_seconds bigint NOT NULL CHECK (oldest_wakeup_age_seconds >= 0),
    pending_due_work_count bigint NOT NULL CHECK (pending_due_work_count >= 0),
    oldest_due_work_age_seconds bigint NOT NULL CHECK (oldest_due_work_age_seconds >= 0),
    expired_delivery_claim_count bigint NOT NULL CHECK (expired_delivery_claim_count >= 0),
    expired_work_lease_count bigint NOT NULL CHECK (expired_work_lease_count >= 0),
    active_reservation_count bigint NOT NULL CHECK (active_reservation_count >= 0),
    retry_exhausted_wakeup_count bigint NOT NULL CHECK (retry_exhausted_wakeup_count >= 0),
    open_quarantine_count bigint NOT NULL CHECK (open_quarantine_count >= 0),
    oldest_open_quarantine_age_seconds bigint NOT NULL CHECK (oldest_open_quarantine_age_seconds >= 0)
);

COMMENT ON TABLE wakeup_telemetry_snapshot IS
    'P04 bounded current telemetry snapshot; diagnostic-only and contains no workflow or provider identifiers.';

-- +goose Down
SELECT 1;
