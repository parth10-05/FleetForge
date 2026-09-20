-- +goose Up
-- Durable physical-operation capacity accounting. Counts change only through
-- explicit, audited reservation resolution; a due-work lease can never alter
-- this row family.
CREATE TABLE scope_capacities (
    scope_level text NOT NULL,
    scope_key varchar(128) NOT NULL,
    capacity_limit bigint NOT NULL,
    active_reservation_count bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (scope_level, scope_key),
    CONSTRAINT scope_capacities_level_known CHECK (scope_level IN ('GLOBAL', 'SITE', 'RACK')),
    CONSTRAINT scope_capacities_key_nonempty CHECK (length(scope_key) > 0),
    CONSTRAINT scope_capacities_global_key CHECK (scope_level <> 'GLOBAL' OR scope_key = 'GLOBAL'),
    CONSTRAINT scope_capacities_limit_positive CHECK (capacity_limit > 0),
    CONSTRAINT scope_capacities_count_nonnegative CHECK (active_reservation_count >= 0),
    CONSTRAINT scope_capacities_count_within_limit CHECK (active_reservation_count <= capacity_limit)
);
COMMENT ON TABLE scope_capacities IS 'P03-Q20: durable GLOBAL/SITE/RACK physical-operation capacity accounting.';

-- +goose Down
SELECT 1;
