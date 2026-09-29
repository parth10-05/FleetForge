package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/platform/config"
)

func TestOpenApplicationPoolSeparatesBoundedPooledSettings(t *testing.T) {
	settings := testSettings()
	pool, err := OpenApplicationPool(context.Background(), settings)
	if err != nil {
		t.Fatalf("OpenApplicationPool() error = %v", err)
	}
	defer pool.Close()
	if got := pool.Config().MaxConns; got != settings.MaxConns {
		t.Fatalf("pool max connections = %d, want %d", got, settings.MaxConns)
	}
	if got := pool.Config().ConnConfig.RuntimeParams["statement_timeout"]; got != "5000" {
		t.Fatalf("statement_timeout = %q, want 5000", got)
	}
}

func TestReadinessProbeClassifiesUnavailableAndIncompatible(t *testing.T) {
	tests := []struct {
		name  string
		probe ReadinessProbe
		want  string
	}{
		{
			name:  "unavailable",
			probe: ReadinessProbe{Ping: func(context.Context) error { return errors.New("private endpoint") }, ReadVersion: func(context.Context) (int64, error) { return 1, nil }, ExpectedVersion: 1},
			want:  "database_unavailable",
		},
		{
			name:  "incompatible",
			probe: ReadinessProbe{Ping: func(context.Context) error { return nil }, ReadVersion: func(context.Context) (int64, error) { return 1, nil }, ExpectedVersion: 2},
			want:  "database_schema_incompatible",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.probe.Check(context.Background())
			if err == nil || test.probe.ReadinessReason(err) != test.want {
				t.Fatalf("Check() error = %v, reason = %q, want %q", err, test.probe.ReadinessReason(err), test.want)
			}
		})
	}
}

func TestMigrationSchemaNameAcceptsOnlySimpleSchemaIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{name: "simple", dsn: "postgres://migrate:secret@127.0.0.1:5432/fleetforge?search_path=ff_it_123", want: "ff_it_123"},
		{name: "missing", dsn: "postgres://migrate:secret@127.0.0.1:5432/fleetforge", want: ""},
		{name: "multiple schemas", dsn: "postgres://migrate:secret@127.0.0.1:5432/fleetforge?search_path=tenant%2Cpublic", want: ""},
		{name: "quoted", dsn: "postgres://migrate:secret@127.0.0.1:5432/fleetforge?search_path=%22tenant%22", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := migrationSchemaName(test.dsn); got != test.want {
				t.Fatalf("migrationSchemaName() = %q, want %q", got, test.want)
			}
		})
	}
}

func testSettings() config.PostgreSQL {
	return config.PostgreSQL{
		ApplicationDSN:     "postgres://app:secret@127.0.0.1:5432/fleetforge?sslmode=disable",
		MigrationDSN:       "postgres://migrate:secret@127.0.0.1:5432/fleetforge?sslmode=disable",
		MaxConns:           4,
		MinConns:           0,
		AcquireTimeout:     time.Second,
		TransactionTimeout: time.Second,
		StatementTimeout:   5 * time.Second,
		MaxConnLifetime:    time.Minute,
		MaxConnIdleTime:    30 * time.Second,
	}
}
