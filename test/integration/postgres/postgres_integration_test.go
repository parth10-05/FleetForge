//go:build postgresintegration

package postgres

import (
	"context"
	"testing"
	"time"
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
