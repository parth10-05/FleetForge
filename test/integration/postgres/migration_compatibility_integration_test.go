//go:build postgresintegration

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/pressly/goose/v3"
)

const lastSupportedMigrationVersion int64 = 5

func TestMigrationCompatibilityEmptyAndLastSupportedReachLatest(t *testing.T) {
	ctx := context.Background()
	freshHarness := New(t)
	if err := persistence.RunMigrations(ctx, integrationSettings(freshHarness.ScopedDSN(t)), migrationFiles()); err != nil {
		t.Fatalf("migrate empty schema: %v", err)
	}
	freshFingerprint := schemaFingerprint(t, freshHarness, ctx)
	assertMigrationVersion(t, freshHarness, ctx, 6)

	upgradeHarness := New(t)
	settings := integrationSettings(upgradeHarness.ScopedDSN(t))
	applyMigrationsThrough(t, ctx, settings, lastSupportedMigrationVersion)
	assertMigrationVersion(t, upgradeHarness, ctx, lastSupportedMigrationVersion)
	if err := persistence.RunMigrations(ctx, settings, migrationFiles()); err != nil {
		t.Fatalf("upgrade last supported schema: %v", err)
	}
	assertMigrationVersion(t, upgradeHarness, ctx, 6)
	if upgradedFingerprint := schemaFingerprint(t, upgradeHarness, ctx); upgradedFingerprint != freshFingerprint {
		t.Fatalf("upgraded schema fingerprint = %s, want %s from empty-to-latest migration", upgradedFingerprint, freshFingerprint)
	}
}

func TestConcurrentMigrationRunnersReachOneStableLatestSchema(t *testing.T) {
	harness := New(t)
	settings := integrationSettings(harness.ScopedDSN(t))
	start := make(chan struct{})
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	run := func() {
		defer wait.Done()
		<-start
		errors <- persistence.RunMigrations(context.Background(), settings, migrationFiles())
	}
	wait.Add(2)
	go run()
	go run()
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent migration runner: %v", err)
		}
	}
	assertMigrationVersion(t, harness, context.Background(), 6)
	if fingerprint := schemaFingerprint(t, harness, context.Background()); fingerprint == "" {
		t.Fatal("latest schema fingerprint is empty")
	}
}

func migrationFiles() fs.FS {
	return os.DirFS(filepath.Join("..", "..", "..", "db", "migrations"))
}

func applyMigrationsThrough(t *testing.T, ctx context.Context, settings config.PostgreSQL, version int64) {
	t.Helper()
	database, err := persistence.OpenMigrationDB(ctx, settings)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	defer database.Close()
	if err := persistence.WithMigrationLock(ctx, database, func(ctx context.Context) error {
		return applyMigrationsThroughLocked(ctx, database, version)
	}); err != nil {
		t.Fatalf("apply migrations through version %d: %v", version, err)
	}
}

func applyMigrationsThroughLocked(ctx context.Context, database *sql.DB, version int64) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrationFiles())
	if err != nil {
		return fmt.Errorf("configure migration provider: %w", err)
	}
	if _, err := provider.UpTo(ctx, version); err != nil {
		return fmt.Errorf("apply migrations through version %d: %w", version, err)
	}
	return nil
}

func assertMigrationVersion(t *testing.T, harness Harness, ctx context.Context, want int64) {
	t.Helper()
	pool := harness.OpenPool(t)
	var got int64
	if err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied").Scan(&got); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if got != want {
		t.Fatalf("migration version = %d, want %d", got, want)
	}
}

func schemaFingerprint(t *testing.T, harness Harness, ctx context.Context) string {
	t.Helper()
	pool := harness.OpenPool(t)
	const catalog = `
WITH definitions AS (
    SELECT 'table:' || table_name AS definition
    FROM information_schema.tables
    WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'
    UNION ALL
    SELECT 'column:' || table_name || ':' || column_name || ':' || data_type || ':' || is_nullable || ':' || COALESCE(column_default, '')
    FROM information_schema.columns
    WHERE table_schema = current_schema()
    UNION ALL
    SELECT 'index:' || indexname || ':' || indexdef
    FROM pg_indexes
    WHERE schemaname = current_schema()
    UNION ALL
    SELECT 'constraint:' || conname || ':' || pg_get_constraintdef(oid)
    FROM pg_constraint
    WHERE connamespace = current_schema()::regnamespace
    UNION ALL
    SELECT 'migration:' || version_id::text || ':' || is_applied::text
    FROM goose_db_version
)
SELECT COALESCE(string_agg(definition, E'\n' ORDER BY definition), '') FROM definitions`
	var definitions string
	if err := pool.QueryRow(ctx, catalog).Scan(&definitions); err != nil {
		t.Fatalf("read schema catalog: %v", err)
	}
	// PostgreSQL includes the generated test schema name in index definitions.
	// The compatibility oracle compares schema shape, not each harness's random
	// namespace, so canonicalize that identifier before hashing.
	definitions = strings.ReplaceAll(definitions, harness.Schema, "<test-schema>")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(definitions))))
}
