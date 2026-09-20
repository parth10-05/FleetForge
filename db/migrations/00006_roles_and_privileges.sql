-- +goose Up
-- P03-15: fixed database roles are deliberately NOLOGIN. Deployment supplies
-- authentication/membership outside this migration; the grants below are the
-- durable least-privilege boundary for every schema created by this migration.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'fleetforge_migration') THEN
        CREATE ROLE fleetforge_migration NOLOGIN NOINHERIT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'fleetforge_application') THEN
        CREATE ROLE fleetforge_application NOLOGIN NOINHERIT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'fleetforge_diagnostic') THEN
        CREATE ROLE fleetforge_diagnostic NOLOGIN NOINHERIT;
    END IF;

    EXECUTE format('GRANT USAGE ON SCHEMA %I TO fleetforge_application, fleetforge_diagnostic', current_schema());
    EXECUTE format('GRANT SELECT, INSERT ON TABLE %I.audit_events TO fleetforge_application', current_schema());
    EXECUTE format('GRANT USAGE, SELECT ON SEQUENCE %I.audit_events_id_seq TO fleetforge_application', current_schema());
    EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA %I TO fleetforge_diagnostic', current_schema());
END
$$;
-- +goose StatementEnd

COMMENT ON ROLE fleetforge_migration IS 'P03-T02: dedicated schema-history and DDL role; invoked only by the migration job.';
COMMENT ON ROLE fleetforge_application IS 'P03-T02: application role; audit append/select only, no schema-history or audit mutation.';
COMMENT ON ROLE fleetforge_diagnostic IS 'P03-T02: read-only diagnostic role.';

-- +goose Down
SELECT 1;
