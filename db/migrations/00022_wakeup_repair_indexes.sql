-- +goose Up
-- P04 repair passes select bounded expired delivery/work leases.  These partial
-- indexes keep the recovery selectors from scanning terminal history.
CREATE INDEX due_work_expired_lease_index
    ON due_work (lease_expires_at, created_at, id)
    WHERE delivery_state = 'CLAIMED';

COMMENT ON INDEX due_work_expired_lease_index IS
    'P04: bounded expired work-lease recovery without terminal-history scans.';

-- +goose Down
SELECT 1;
