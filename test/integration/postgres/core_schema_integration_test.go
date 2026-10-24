//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
)

func TestCoreSchemaRejectsInvalidStateCountersOrdinalsAndForeignKeys(t *testing.T) {
	harness := New(t)
	settings := integrationSettings(harness.ScopedDSN(t))
	if err := persistence.RunMigrations(context.Background(), settings, os.DirFS(filepath.Join("..", "..", "..", "db", "migrations"))); err != nil {
		t.Fatalf("migrate core schema: %v", err)
	}
	pool := harness.OpenPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assertConstraint(t, pool, ctx, "INSERT INTO targets (id, external_identity, endpoint_reference, site_key, rack_key, administrative_state) VALUES ('00000000-0000-0000-0000-000000000001', 'target-1', 'secret-ref', 'site-a', 'rack-a', 'UNKNOWN')")
	mustExecute(t, pool, ctx, "INSERT INTO firmware_artifacts (id, firmware_version, sha256_digest, object_reference, state) VALUES ('00000000-0000-0000-0000-000000000010', 'v1', 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'artifact-ref', 'VERIFIED')")
	assertConstraint(t, pool, ctx, "INSERT INTO campaigns (id, artifact_id, state, revision, command_epoch, policy_version_id) VALUES ('00000000-0000-0000-0000-000000000020', '00000000-0000-0000-0000-000000000010', 'DRAFT', -1, 0, '00000000-0000-0000-0000-000000000021')")
	mustExecute(t, pool, ctx, "INSERT INTO campaigns (id, artifact_id, state, policy_version_id) VALUES ('00000000-0000-0000-0000-000000000022', '00000000-0000-0000-0000-000000000010', 'DRAFT', '00000000-0000-0000-0000-000000000023')")
	mustExecute(t, pool, ctx, "INSERT INTO campaign_waves (id, campaign_id, ordinal, evaluation, terminal) VALUES ('00000000-0000-0000-0000-000000000030', '00000000-0000-0000-0000-000000000022', 1, 'PASSED', true)")
	assertSQLState(t, pool, ctx, "INSERT INTO campaign_waves (id, campaign_id, ordinal, evaluation, terminal) VALUES ('00000000-0000-0000-0000-000000000031', '00000000-0000-0000-0000-000000000022', 1, 'PASSED', true)", "23505")
	assertSQLState(t, pool, ctx, "INSERT INTO campaign_waves (id, campaign_id, ordinal, evaluation, terminal) VALUES ('00000000-0000-0000-0000-000000000032', '00000000-0000-0000-0000-000000000099', 1, 'PASSED', true)", "23503")
}

func mustExecute(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, ctx context.Context, statement string) {
	t.Helper()
	if _, err := pool.Exec(ctx, statement); err != nil {
		t.Fatalf("seed statement failed: %v", err)
	}
}

func assertConstraint(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, ctx context.Context, statement string) {
	t.Helper()
	_, err := pool.Exec(ctx, statement)
	if err == nil {
		t.Fatalf("statement unexpectedly succeeded: %s", statement)
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || (postgresError.Code != "23514" && postgresError.Code != "23503") {
		t.Fatalf("statement error = %v, want PostgreSQL check or foreign-key violation", err)
	}
}

func assertSQLState(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, ctx context.Context, statement, want string) {
	t.Helper()
	_, err := pool.Exec(ctx, statement)
	var postgresError *pgconn.PgError
	if err == nil || !errors.As(err, &postgresError) || postgresError.Code != want {
		t.Fatalf("statement error = %v, want PostgreSQL SQLSTATE %s", err, want)
	}
}

func integrationSettings(dsn string) config.PostgreSQL {
	// The tagged integration suite may target a scale-to-zero remote PostgreSQL
	// branch. Allow one bounded cold-start/TLS handshake without changing the
	// production role's much tighter normal acquire budget.
	return config.PostgreSQL{ApplicationDSN: dsn, MigrationDSN: dsn, MaxConns: 2, AcquireTimeout: 15 * time.Second, TransactionTimeout: 10 * time.Second, StatementTimeout: 5 * time.Second, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute}
}
