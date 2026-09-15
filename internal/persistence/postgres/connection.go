// Package postgres owns PostgreSQL connectivity, migration serialization, and
// readiness classification. Persistence mappers and repositories are added in
// later Phase 03 cards.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/pressly/goose/v3"
)

const migrationAdvisoryLockKey int64 = 6_037_184_357_901_114_221

var (
	ErrDatabaseUnavailable        = errors.New("database unavailable")
	ErrDatabaseSchemaIncompatible = errors.New("database schema incompatible")
)

// OpenApplicationPool creates the bounded pooled path used by application
// roles. Construction is lazy; callers choose when a persistence-aware role
// should contact PostgreSQL.
func OpenApplicationPool(ctx context.Context, settings config.PostgreSQL) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(settings.ApplicationDSN)
	if err != nil {
		return nil, fmt.Errorf("parse application database configuration: %w", err)
	}
	poolConfig.MaxConns = settings.MaxConns
	poolConfig.MinConns = settings.MinConns
	poolConfig.MaxConnLifetime = settings.MaxConnLifetime
	poolConfig.MaxConnIdleTime = settings.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = min(settings.MaxConnIdleTime/2, time.Minute)
	poolConfig.ConnConfig.ConnectTimeout = settings.AcquireTimeout
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(settings.StatementTimeout.Milliseconds(), 10)
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open application database pool: %w", err)
	}
	return pool, nil
}

// OpenMigrationDB creates the direct, one-session path for migrations and
// session advisory locks. It must not receive the pooled application DSN.
func OpenMigrationDB(ctx context.Context, settings config.PostgreSQL) (*sql.DB, error) {
	connectionConfig, err := pgx.ParseConfig(settings.MigrationDSN)
	if err != nil {
		return nil, fmt.Errorf("parse migration database configuration: %w", err)
	}
	database := stdlib.OpenDB(*connectionConfig)
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(settings.MaxConnLifetime)
	database.SetConnMaxIdleTime(settings.MaxConnIdleTime)
	pingContext, cancel := context.WithTimeout(ctx, settings.AcquireTimeout)
	defer cancel()
	if err := database.PingContext(pingContext); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("%w: direct migration connection", ErrDatabaseUnavailable)
	}
	return database, nil
}

// WithMigrationLock serializes migration runners with a PostgreSQL session
// advisory lock. The one-connection migration database keeps the session lock
// on the exact connection goose subsequently uses.
func WithMigrationLock(ctx context.Context, database *sql.DB, run func(context.Context) error) (result error) {
	if _, err := database.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("%w: acquire migration advisory lock", ErrDatabaseUnavailable)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := database.ExecContext(unlockContext, "SELECT pg_advisory_unlock($1)", migrationAdvisoryLockKey); err != nil && result == nil {
			result = fmt.Errorf("%w: release migration advisory lock", ErrDatabaseUnavailable)
		}
	}()
	return run(ctx)
}

// RunMigrations applies immutable SQL migrations through goose only after a
// direct-session lock is held. Application binaries never invoke this path.
func RunMigrations(ctx context.Context, settings config.PostgreSQL, migrations fs.FS) error {
	database, err := OpenMigrationDB(ctx, settings)
	if err != nil {
		return err
	}
	defer database.Close()
	return WithMigrationLock(ctx, database, func(ctx context.Context) error {
		provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations)
		if err != nil {
			return fmt.Errorf("configure migration provider: %w", err)
		}
		// Provider.Close closes the caller-supplied *sql.DB. The database must
		// remain open until WithMigrationLock releases its session advisory lock;
		// RunMigrations owns and closes it after that scope returns.
		if _, err := provider.Up(ctx); err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
		return nil
	})
}

// VersionReader obtains the migration version that an application requires.
// It is injected so readiness can be tested without a live database.
type VersionReader func(context.Context) (int64, error)

// ReadinessProbe classifies connectivity separately from an incompatible
// migration version. Its safe reason is consumed by healthhttp.ReasonedProbe.
type ReadinessProbe struct {
	Ping            func(context.Context) error
	ReadVersion     VersionReader
	ExpectedVersion int64
}

func (probe ReadinessProbe) Check(ctx context.Context) error {
	if probe.Ping == nil || probe.ReadVersion == nil || probe.ExpectedVersion < 0 {
		return ErrDatabaseSchemaIncompatible
	}
	if err := probe.Ping(ctx); err != nil {
		return fmt.Errorf("%w: ping", ErrDatabaseUnavailable)
	}
	version, err := probe.ReadVersion(ctx)
	if err != nil || version != probe.ExpectedVersion {
		return fmt.Errorf("%w: expected migration version", ErrDatabaseSchemaIncompatible)
	}
	return nil
}

func (probe ReadinessProbe) ReadinessReason(err error) string {
	if errors.Is(err, ErrDatabaseSchemaIncompatible) {
		return "database_schema_incompatible"
	}
	return "database_unavailable"
}
