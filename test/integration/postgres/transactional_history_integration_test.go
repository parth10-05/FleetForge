//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestConcurrentAdmissionLeavesOneCoherentDurableHistory(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)

	requests := []persistence.AdmissionRequest{
		admissionRequest(fixture, "1301"),
		admissionRequest(fixture, "1302"),
	}
	start := make(chan struct{})
	results := make(chan error, len(requests))
	var wait sync.WaitGroup
	for _, request := range requests {
		wait.Add(1)
		go func(request persistence.AdmissionRequest) {
			defer wait.Done()
			<-start
			results <- persistence.AdmitOperation(context.Background(), pool, request)
		}(request)
	}
	close(start)
	wait.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		if !errors.Is(err, persistence.ErrRevisionConflict) && !errors.Is(err, persistence.ErrAdmissionBlocked) && !errors.Is(err, persistence.ErrAdmissionDuplicate) {
			t.Fatalf("competing admission error = %v, want a bounded stale/duplicate outcome", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful concurrent admissions = %d, want exactly one", succeeded)
	}

	assertHistoryCount(t, pool, ctx, "submission_admissions", 1)
	assertHistoryCount(t, pool, ctx, "operation_reservations", 3)
	assertHistoryCount(t, pool, ctx, "step_attempts", 1)
	assertHistoryCount(t, pool, ctx, "decision_inputs", 1)
	assertHistoryCount(t, pool, ctx, "audit_events", 1)
	assertHistoryCount(t, pool, ctx, "outbox_events", 2)
	assertHistoryCount(t, pool, ctx, "due_work", 1)
	var active int64
	if err := pool.QueryRow(ctx, "SELECT COALESCE(SUM(active_reservation_count),0) FROM scope_capacities").Scan(&active); err != nil {
		t.Fatalf("read reservation capacity: %v", err)
	}
	if active != 3 {
		t.Fatalf("active reservation capacity = %d, want 3 from the one committed admission", active)
	}
	// Independent direct-row oracle: every committed admission evidence row
	// shares the winning aggregate revision and audit identity is explicit.
	var auditRevision int64
	var correlation, causation string
	if err := pool.QueryRow(ctx, "SELECT aggregate_revision,correlation_id::text,causation_id::text FROM audit_events WHERE aggregate_type='TARGET_UPDATE'").Scan(&auditRevision, &correlation, &causation); err != nil {
		t.Fatalf("read audit history oracle: %v", err)
	}
	if auditRevision != 1 || correlation == "" || causation == "" {
		t.Fatalf("audit history=%d/%q/%q, want revision 1 and immutable identities", auditRevision, correlation, causation)
	}
	var dueRevision int64
	if err := pool.QueryRow(ctx, "SELECT expected_revision FROM due_work").Scan(&dueRevision); err != nil || dueRevision != auditRevision {
		t.Fatalf("due-work revision=%d err=%v, want audit revision %d", dueRevision, err, auditRevision)
	}
}

func assertHistoryCount(t *testing.T, pool *pgxpool.Pool, ctx context.Context, table string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
