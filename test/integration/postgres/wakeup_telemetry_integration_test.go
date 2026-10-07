//go:build postgresintegration

package postgres

import (
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestWakeupTelemetryPersistsOneIdentifierFreeCurrentSnapshot(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	firstAt := time.Date(2026, 10, 7, 1, 0, 0, 0, time.UTC)
	first, err := persistence.CollectWakeupTelemetry(ctx, pool, firstAt)
	if err != nil {
		t.Fatal(err)
	}
	if first.ObservedAt != firstAt || first.PendingWakeupCount != 0 || first.PendingDueWorkCount != 0 || first.OpenQuarantineCount != 0 || first.OldestWakeupAgeSeconds != 0 || first.OldestDueWorkAgeSeconds != 0 || first.OldestOpenQuarantineAgeSeconds != 0 {
		t.Fatalf("empty snapshot=%#v", first)
	}
	secondAt := firstAt.Add(time.Minute)
	if _, err := persistence.CollectWakeupTelemetry(ctx, pool, secondAt); err != nil {
		t.Fatal(err)
	}
	var rows int
	var observed time.Time
	if err := pool.QueryRow(ctx, "SELECT COUNT(*), MAX(observed_at) FROM wakeup_telemetry_snapshot").Scan(&rows, &observed); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || !observed.Equal(secondAt) {
		t.Fatalf("snapshot rows=%d observed=%s want one row at %s", rows, observed, secondAt)
	}
}
