// Package postgres provides a CI-only real PostgreSQL harness shared by integration and composed fault tests.
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TestDSNEnvironment = "FLEETFORGE_TEST_POSTGRES_DSN"

type Harness struct{ DSN, Schema string }

func New(t testing.TB) Harness {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(TestDSNEnvironment))
	if dsn == "" {
		t.Fatalf("%s is required for the PostgreSQL integration gate", TestDSNEnvironment)
	}
	identifier := makeIdentifier(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect PostgreSQL integration database: %v", err)
	}
	if _, err := connection.Exec(ctx, "CREATE SCHEMA "+quoteIdentifier(identifier)); err != nil {
		connection.Close(context.Background())
		t.Fatalf("create isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.Exec(cleanupContext, "DROP SCHEMA "+quoteIdentifier(identifier)+" CASCADE")
		_ = connection.Close(cleanupContext)
	})
	return Harness{DSN: dsn, Schema: identifier}
}

func (h Harness) OpenPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	poolConfig, err := pgxpool.ParseConfig(h.DSN)
	if err != nil {
		t.Fatalf("parse PostgreSQL integration configuration: %v", err)
	}
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = map[string]string{}
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = h.Schema
	poolConfig.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		if _, err := connection.Exec(ctx, "SELECT set_config('search_path', $1, false)", h.Schema); err != nil {
			return fmt.Errorf("configure isolated PostgreSQL test schema: %w", err)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("open PostgreSQL integration pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func (h Harness) ScopedDSN(t testing.TB) string {
	t.Helper()
	parsed, err := url.Parse(h.DSN)
	if err != nil {
		t.Fatalf("parse PostgreSQL integration DSN: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", h.Schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func makeIdentifier(t testing.TB) string {
	t.Helper()
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatalf("generate PostgreSQL test schema identifier: %v", err)
	}
	return "ff_it_" + hex.EncodeToString(bytes)
}
func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
func (h Harness) String() string { return fmt.Sprintf("isolated schema %s", h.Schema) }
