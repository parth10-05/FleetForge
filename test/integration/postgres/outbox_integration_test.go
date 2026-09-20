//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestOutboxClaimPublishesAfterCommitAndFencesStalePublisher(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Now().UTC()
	insertOutbox(t, pool, ctx, "00000000-0000-0000-1201-000000000001", now.Add(-time.Minute))
	claimed, err := persistence.ClaimOutbox(ctx, pool, persistence.ClaimOutboxRequest{Owner: "publisher-a", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{"00000000-0000-0000-1201-000000000002"}})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%v err=%v, want one", claimed, err)
	}
	if err := persistence.MarkOutboxPublished(ctx, pool, persistence.OutboxResult{EventID: claimed[0].EventID, ClaimToken: "00000000-0000-0000-1201-000000000003", Now: now}); !errors.Is(err, persistence.ErrOutboxClaimConflict) {
		t.Fatalf("stale mark=%v, want conflict", err)
	}
	if err := persistence.MarkOutboxPublished(ctx, pool, persistence.OutboxResult{EventID: claimed[0].EventID, ClaimToken: claimed[0].ClaimToken, Now: now}); err != nil {
		t.Fatalf("mark published: %v", err)
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM outbox_events WHERE event_id=$1", claimed[0].EventID).Scan(&state); err != nil || state != "PUBLISHED" {
		t.Fatalf("state=%q err=%v, want PUBLISHED", state, err)
	}
}

func TestOutboxRetryQuarantinesAtBudget(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Now().UTC()
	insertOutbox(t, pool, ctx, "00000000-0000-0000-1202-000000000001", now.Add(-time.Minute))
	claimed, err := persistence.ClaimOutbox(ctx, pool, persistence.ClaimOutboxRequest{Owner: "publisher-a", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{"00000000-0000-0000-1202-000000000002"}})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	if err := persistence.RetryOrQuarantineOutbox(ctx, pool, persistence.OutboxResult{EventID: claimed[0].EventID, ClaimToken: claimed[0].ClaimToken, Now: now, RetryAt: now.Add(time.Minute), ErrorClass: "PUBLISH_FAILED", MaxAttempts: 1}); err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM outbox_events WHERE event_id=$1", claimed[0].EventID).Scan(&state); err != nil || state != "QUARANTINED" {
		t.Fatalf("state=%q err=%v, want QUARANTINED", state, err)
	}
}

func TestOutboxSuccessThenCrashBeforeMarkIsReclaimedAtLeastOnce(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	insertOutbox(t, pool, ctx, "00000000-0000-0000-1203-000000000001", now.Add(-time.Minute))
	first, err := persistence.ClaimOutbox(ctx, pool, persistence.ClaimOutboxRequest{Owner: "publisher-a", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{"00000000-0000-0000-1203-000000000002"}})
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim=%v err=%v", first, err)
	}
	// The successful external call is deliberately represented by this durable
	// test seam; process termination happens before MarkOutboxPublished.
	delivered := first[0].EventID
	repaired, err := persistence.RepairExpiredOutbox(ctx, pool, now.Add(2*time.Minute), 1)
	if err != nil || len(repaired) != 1 || repaired[0] != delivered {
		t.Fatalf("repair=%v err=%v", repaired, err)
	}
	second, err := persistence.ClaimOutbox(ctx, pool, persistence.ClaimOutboxRequest{Owner: "publisher-b", Now: now.Add(2 * time.Minute), LeaseExpiresAt: now.Add(3 * time.Minute), Tokens: []string{"00000000-0000-0000-1203-000000000003"}})
	if err != nil || len(second) != 1 || second[0].EventID != delivered {
		t.Fatalf("reclaim=%v err=%v; want redelivery of %s", second, err, delivered)
	}
}

func insertOutbox(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id string, next time.Time) {
	t.Helper()
	if _, err := pool.Exec(ctx, "INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload,next_attempt_at) VALUES ($1::uuid,'CAMPAIGN','00000000-0000-0000-1200-000000000001',0,'campaign.ready',1,'{}',$2)", id, next); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
}
