//go:build postgresintegration

package v1demo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	testpostgres "github.com/parth10-05/fleetforge/test/support/postgres"
)

func acceptanceHarness(t *testing.T) (testpostgres.Harness, *pgxpool.Pool, context.Context, string) {
	t.Helper()
	harness := testpostgres.New(t)
	root := acceptanceRepositoryRoot(t)
	settings := config.PostgreSQL{ApplicationDSN: harness.ScopedDSN(t), MigrationDSN: harness.ScopedDSN(t), MaxConns: 4, AcquireTimeout: configDuration("15s"), TransactionTimeout: configDuration("10s"), StatementTimeout: configDuration("5s")}
	if err := persistence.RunMigrations(context.Background(), settings, os.DirFS(filepath.Join(root, "db", "migrations"))); err != nil {
		t.Fatalf("migrate P06 acceptance schema: %v", err)
	}
	return harness, harness.OpenPool(t), context.Background(), root
}

func acceptanceRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func configDuration(value string) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil {
		panic(err)
	}
	return duration
}
