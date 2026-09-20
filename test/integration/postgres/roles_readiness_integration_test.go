//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestRolesEnforceAuditAndDiagnosticBoundaries(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	if _, err := pool.Exec(ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,correlation_id,causation_id,action,reason_code) VALUES ('CAMPAIGN','00000000-0000-0000-1500-000000000001',0,0,'role-test','00000000-0000-0000-1500-000000000011','00000000-0000-0000-1500-000000000011','TEST','OK')"); err != nil {
		t.Fatalf("seed audit row: %v", err)
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire role test connection: %v", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "SET SESSION AUTHORIZATION fleetforge_application"); err != nil {
		t.Fatalf("assume application role: %v", err)
	}
	if _, err := connection.Exec(ctx, "UPDATE audit_events SET action='MUTATED'"); err == nil {
		t.Fatal("application role updated audit history")
	}
	if _, err := connection.Exec(ctx, "DELETE FROM audit_events"); err == nil {
		t.Fatal("application role deleted audit history")
	}
	if _, err := connection.Exec(ctx, "UPDATE goose_db_version SET version_id=0"); err == nil {
		t.Fatal("application role mutated schema history")
	}
	if _, err := connection.Exec(ctx, "RESET SESSION AUTHORIZATION"); err != nil {
		t.Fatalf("reset application role: %v", err)
	}
	if _, err := connection.Exec(ctx, "SET SESSION AUTHORIZATION fleetforge_diagnostic"); err != nil {
		t.Fatalf("assume diagnostic role: %v", err)
	}
	if _, err := connection.Exec(ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,correlation_id,causation_id,action,reason_code) VALUES ('CAMPAIGN','00000000-0000-0000-1500-000000000002',0,0,'diagnostic','00000000-0000-0000-1500-000000000012','00000000-0000-0000-1500-000000000012','TEST','OK')"); err == nil {
		t.Fatal("diagnostic role mutated operational data")
	}
}

func TestReadinessProbeSeparatesUnavailableFromSchemaMismatch(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	probe := persistence.ReadinessProbe{
		Ping: pool.Ping,
		ReadVersion: func(ctx context.Context) (int64, error) {
			var version int64
			err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied").Scan(&version)
			return version, err
		},
		ExpectedVersion: 7,
	}
	if err := probe.Check(ctx); err != nil {
		t.Fatalf("compatible schema readiness: %v", err)
	}
	probe.ExpectedVersion = 7
	err := probe.Check(ctx)
	if !errors.Is(err, persistence.ErrDatabaseSchemaIncompatible) || probe.ReadinessReason(err) != "database_schema_incompatible" {
		t.Fatalf("schema mismatch = %v, want incompatible classification", err)
	}
	probe.Ping = func(context.Context) error { return pgx.ErrTxClosed }
	err = probe.Check(ctx)
	if !errors.Is(err, persistence.ErrDatabaseUnavailable) || probe.ReadinessReason(err) != "database_unavailable" {
		t.Fatalf("unavailable readiness = %v, want unavailable classification", err)
	}
}
