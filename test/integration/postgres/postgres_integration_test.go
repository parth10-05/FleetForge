//go:build postgresintegration

package postgres

import (
	"context"
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestDatabasePingUsesIsolatedSchema(t *testing.T) {
	harness := New(t)
	pool := harness.OpenPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL integration database: %v", err)
	}
	var currentSchema string
	if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&currentSchema); err != nil {
		t.Fatalf("read isolated PostgreSQL schema: %v", err)
	}
	if currentSchema != harness.Schema {
		t.Fatalf("current schema = %q, want %q", currentSchema, harness.Schema)
	}
}

func TestMigrationConnectionUsesIsolatedSchema(t *testing.T) {
	harness := New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	database, err := persistence.OpenMigrationDB(ctx, integrationSettings(harness.ScopedDSN(t)))
	if err != nil {
		t.Fatalf("open isolated migration connection: %v", err)
	}
	defer database.Close()

	var currentSchema string
	if err := database.QueryRowContext(ctx, "SELECT current_schema()").Scan(&currentSchema); err != nil {
		t.Fatalf("read migration connection schema: %v", err)
	}
	if currentSchema != harness.Schema {
		t.Fatalf("migration connection schema = %q, want %q", currentSchema, harness.Schema)
	}
}
