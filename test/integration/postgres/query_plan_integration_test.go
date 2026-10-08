//go:build postgresintegration

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestOldestWorkQueriesUseDocumentedIndexesAtThousandTargetScale emits the
// complete EXPLAIN ANALYZE/BUFFERS plan on failure and keeps the 1,000-record
// workload reproducible through fixed identifiers and timestamps.
func TestOldestWorkQueriesUseDocumentedIndexesAtThousandTargetScale(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("00000000-0000-0000-1600-%012d", i+1)
		command := fmt.Sprintf("00000000-0000-0000-1601-%012d", i+1)
		if _, err := pool.Exec(ctx, "INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision) VALUES ($1::uuid,$2::uuid,'TARGET_UPDATE','00000000-0000-0000-1602-000000000001','PRECHECK',1,$3,0)", id, command, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed due %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload,next_attempt_at) VALUES ($1::uuid,'CAMPAIGN','00000000-0000-0000-1603-000000000001',0,'campaign.ready',1,'{}',$2)", id, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed outbox %d: %v", i, err)
		}
	}
	assertPlanUsesIndex(t, ctx, pool, "EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM due_work WHERE delivery_state='PENDING' ORDER BY not_before,created_at,id LIMIT 10", "due_work_oldest_pending_index")
	assertPlanUsesIndex(t, ctx, pool, "EXPLAIN (ANALYZE, BUFFERS) SELECT event_id FROM outbox_events WHERE delivery_state='PENDING' ORDER BY next_attempt_at,created_at,event_id LIMIT 10", "outbox_events_oldest_pending_index")
	// These are the P04 production selector shapes, not proxy index checks.
	assertPlanUsesIndex(t, ctx, pool, "EXPLAIN (ANALYZE, BUFFERS) SELECT event_id FROM outbox_events WHERE delivery_state='PENDING' AND delivery_kind='WORK_WAKEUP' AND next_attempt_at <= TIMESTAMPTZ '2026-09-21 00:00:00+00' ORDER BY next_attempt_at,created_at,event_id LIMIT 10", "outbox_events_work_wakeup_publishable_index")
	assertPlanUsesIndex(t, ctx, pool, "EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM due_work WHERE delivery_state='CLAIMED' AND lease_expires_at <= TIMESTAMPTZ '2026-09-21 00:00:00+00' ORDER BY lease_expires_at,created_at,id LIMIT 10", "due_work_expired_lease_index")
	assertPlanUsesIndex(t, ctx, pool, "EXPLAIN (ANALYZE, BUFFERS) SELECT event_id FROM outbox_events WHERE delivery_state='CLAIMED' AND claim_expires_at <= TIMESTAMPTZ '2026-09-21 00:00:00+00' ORDER BY claim_expires_at,created_at,event_id LIMIT 10", "outbox_events_expired_claim_index")
	assertPlanUsesIndex(t, ctx, pool, "EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM due_work WHERE delivery_state='PENDING' AND not_before <= TIMESTAMPTZ '2026-09-21 00:00:00+00' ORDER BY not_before,created_at,id LIMIT 10", "due_work_oldest_pending_index")
}

func assertPlanUsesIndex(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql, index string) {
	t.Helper()
	rows, err := pool.Query(ctx, sql)
	if err != nil {
		t.Fatalf("explain %s: %v", index, err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan %s plan: %v", index, err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read %s plan: %v", index, err)
	}
	if !strings.Contains(plan.String(), index) {
		t.Fatalf("plan does not use %s:\n%s", index, plan.String())
	}
}
