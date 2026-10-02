//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestDueWorkClaimFencesStaleCompletion(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Now().UTC()
	insertDueWork(t, pool, ctx, "00000000-0000-0000-1101-000000000001", "00000000-0000-0000-1101-000000000002", fixture.targetUpdateID, "PRECHECK", now.Add(-time.Minute))
	claimed, err := persistence.ClaimDueWork(ctx, pool, persistence.ClaimDueWorkRequest{Owner: "worker-a", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{"00000000-0000-0000-1101-000000000003"}})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%v err=%v, want one claim", claimed, err)
	}
	wrong := persistence.DueWorkResult{ID: claimed[0].ID, LeaseToken: "00000000-0000-0000-1101-000000000004", ExpectedRevision: 0, Now: now}
	if err := persistence.CompleteDueWork(ctx, pool, wrong); !errors.Is(err, persistence.ErrLeaseConflict) {
		t.Fatalf("wrong-token completion=%v, want ErrLeaseConflict", err)
	}
	right := persistence.DueWorkResult{ID: claimed[0].ID, LeaseToken: claimed[0].LeaseToken, ExpectedRevision: claimed[0].ExpectedRevision, ExpectedCommandEpoch: claimed[0].ExpectedCommandEpoch, Now: now}
	if err := persistence.CompleteDueWork(ctx, pool, right); err != nil {
		t.Fatalf("complete due work: %v", err)
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1", claimed[0].ID).Scan(&state); err != nil || state != "COMPLETED" {
		t.Fatalf("state=%q err=%v, want COMPLETED", state, err)
	}
}

func TestExpiredSubmitRepairRequiresReconciliationAndRetainsReservation(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	insertAdmission(t, pool, ctx, "00000000-0000-0000-1102-000000000001", "00000000-0000-0000-1102-000000000002", fixture)
	if _, err := pool.Exec(ctx, "INSERT INTO operation_reservations (id,admission_id,target_id,campaign_id,scope_level,scope_key) VALUES ('00000000-0000-0000-1102-000000000003',$1,$2,$3,'GLOBAL','GLOBAL')", "00000000-0000-0000-1102-000000000001", fixture.targetID, fixture.campaignID); err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	now := time.Now().UTC()
	insertDueWork(t, pool, ctx, "00000000-0000-0000-1102-000000000004", "00000000-0000-0000-1102-000000000005", fixture.targetUpdateID, "SUBMIT", now.Add(-time.Hour))
	if _, err := pool.Exec(ctx, "UPDATE due_work SET delivery_state='CLAIMED',lease_token='00000000-0000-0000-1102-000000000006',lease_owner='lost',lease_expires_at=$2 WHERE id=$1", "00000000-0000-0000-1102-000000000004", now.Add(-time.Minute)); err != nil {
		t.Fatalf("expire work lease: %v", err)
	}
	repaired, err := persistence.RepairExpiredDueWork(ctx, pool, now, 1)
	if err != nil || len(repaired) != 1 {
		t.Fatalf("repair=%v err=%v, want one", repaired, err)
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1", repaired[0]).Scan(&state); err != nil || state != "RECONCILIATION_REQUIRED" {
		t.Fatalf("work state=%q err=%v, want RECONCILIATION_REQUIRED", state, err)
	}
	var reservations int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM operation_reservations WHERE state='ACTIVE'").Scan(&reservations); err != nil || reservations != 1 {
		t.Fatalf("active reservations=%d err=%v, want 1", reservations, err)
	}
	stale := persistence.DueWorkResult{ID: repaired[0], LeaseToken: "00000000-0000-0000-1102-000000000006", ExpectedRevision: 0, Now: now}
	if err := persistence.CompleteDueWork(ctx, pool, stale); !errors.Is(err, persistence.ErrLeaseConflict) {
		t.Fatalf("live stale worker completion=%v, want ErrLeaseConflict", err)
	}
}

// TestDueWorkCompletionRetryAndRepairRacePreservesFencing is P03-IT-17's
// persistence-level schedule. A stale worker can neither complete nor retry
// work after the sweeper observes its expired lease; no path releases the
// durable operation reservation.
func TestDueWorkCompletionRetryAndRepairRacePreservesFencing(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	insertAdmission(t, pool, ctx, "00000000-0000-0000-1120-000000000001", "00000000-0000-0000-1120-000000000002", fixture)
	if _, err := pool.Exec(ctx, "INSERT INTO operation_reservations (id,admission_id,target_id,campaign_id,scope_level,scope_key) VALUES ('00000000-0000-0000-1120-000000000003',$1,$2,$3,'GLOBAL','GLOBAL')", "00000000-0000-0000-1120-000000000001", fixture.targetID, fixture.campaignID); err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	now := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	insertDueWork(t, pool, ctx, "00000000-0000-0000-1120-000000000004", "00000000-0000-0000-1120-000000000005", fixture.targetUpdateID, "PRECHECK", now.Add(-time.Hour))
	claimed, err := persistence.ClaimDueWork(ctx, pool, persistence.ClaimDueWorkRequest{Owner: "stale-worker", Now: now.Add(-time.Minute), LeaseExpiresAt: now, Tokens: []string{"00000000-0000-0000-1120-000000000006"}})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim stale work=%v err=%v, want one", claimed, err)
	}
	result := persistence.DueWorkResult{ID: claimed[0].ID, LeaseToken: claimed[0].LeaseToken, ExpectedRevision: claimed[0].ExpectedRevision, ExpectedCommandEpoch: claimed[0].ExpectedCommandEpoch, Now: now, RetryAt: now.Add(time.Minute)}
	start := make(chan struct{})
	errs := make(chan error, 3)
	go func() { <-start; errs <- persistence.CompleteDueWork(context.Background(), pool, result) }()
	go func() { <-start; errs <- persistence.FailDueWork(context.Background(), pool, result) }()
	go func() {
		<-start
		_, err := persistence.RepairExpiredDueWork(context.Background(), pool, now, 1)
		errs <- err
	}()
	close(start)
	for i := 0; i < 3; i++ {
		if err := <-errs; err != nil && !errors.Is(err, persistence.ErrLeaseConflict) {
			t.Fatalf("completion/retry/repair race error=%v", err)
		}
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1", result.ID).Scan(&state); err != nil {
		t.Fatalf("read raced work: %v", err)
	}
	if state != "COMPLETED" && state != "PENDING" {
		t.Fatalf("raced work state=%q, want COMPLETED or PENDING", state)
	}
	var active int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM operation_reservations WHERE state='ACTIVE'").Scan(&active); err != nil || active != 1 {
		t.Fatalf("active reservations=%d err=%v, want 1", active, err)
	}
}

func TestConcurrentDueWorkClaimersDoNotCoOwnLease(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Now().UTC()
	insertDueWork(t, pool, ctx, "00000000-0000-0000-1103-000000000001", "00000000-0000-0000-1103-000000000002", fixture.targetUpdateID, "PRECHECK", now.Add(-time.Minute))
	results := make(chan []persistence.ClaimedDueWork, 2)
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for i, token := range []string{"00000000-0000-0000-1103-000000000003", "00000000-0000-0000-1103-000000000004"} {
		wait.Add(1)
		go func(owner, leaseToken string) {
			defer wait.Done()
			rows, err := persistence.ClaimDueWork(context.Background(), pool, persistence.ClaimDueWorkRequest{Owner: owner, Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{leaseToken}})
			results <- rows
			errs <- err
		}("worker-"+string(rune('a'+i)), token)
	}
	wait.Wait()
	close(results)
	close(errs)
	count := 0
	for err := range errs {
		if err != nil && !errors.Is(err, persistence.ErrLeaseConflict) {
			t.Fatalf("concurrent claim: %v", err)
		}
	}
	for rows := range results {
		count += len(rows)
	}
	if count != 1 {
		t.Fatalf("claimed rows=%d, want exactly one", count)
	}
}

// TestTenConcurrentDueWorkClaimersLeaveOneFencedOwnerPerRow is P03-IT-06.
func TestTenConcurrentDueWorkClaimersLeaveOneFencedOwnerPerRow(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		insertDueWork(t, pool, ctx, fmt.Sprintf("00000000-0000-0000-1110-%012d", i+1), fmt.Sprintf("00000000-0000-0000-1111-%012d", i+1), fixture.targetUpdateID, "PRECHECK", now.Add(time.Duration(i)*time.Second))
	}
	type result struct {
		rows []persistence.ClaimedDueWork
		err  error
	}
	results, start := make(chan result, 10), make(chan struct{})
	var wait sync.WaitGroup
	for i := 0; i < 10; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			<-start
			req := persistence.ClaimDueWorkRequest{Owner: fmt.Sprintf("worker-%02d", i+1), Now: now.Add(time.Minute), LeaseExpiresAt: now.Add(2 * time.Minute), Tokens: []string{fmt.Sprintf("00000000-0000-0000-1112-%012d", i+1)}}
			var rows []persistence.ClaimedDueWork
			var err error
			for attempt := 0; attempt < 100; attempt++ {
				rows, err = persistence.ClaimDueWork(context.Background(), pool, req)
				if !errors.Is(err, persistence.ErrLeaseConflict) {
					break
				}
				time.Sleep(time.Duration(attempt+1) * time.Millisecond)
			}
			results <- result{rows, err}
		}(i)
	}
	close(start)
	wait.Wait()
	close(results)
	seen := map[string]string{}
	for result := range results {
		if result.err != nil {
			t.Fatalf("ten-claimer schedule: %v", result.err)
		}
		for _, row := range result.rows {
			if prior, exists := seen[row.ID]; exists {
				t.Fatalf("due work %s co-owned by %s and %s", row.ID, prior, row.LeaseToken)
			}
			seen[row.ID] = row.LeaseToken
		}
	}
	if len(seen) != 10 {
		t.Fatalf("unique claimed rows=%d, want 10", len(seen))
	}
	var claimed int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE delivery_state='CLAIMED' AND lease_token IS NOT NULL AND lease_owner IS NOT NULL").Scan(&claimed); err != nil || claimed != 10 {
		t.Fatalf("fenced claimed rows=%d err=%v, want 10", claimed, err)
	}
}

func insertDueWork(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id, commandID, subjectID, workType string, notBefore time.Time) {
	t.Helper()
	if _, err := pool.Exec(ctx, "INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision) VALUES ($1::uuid,$2::uuid,'TARGET_UPDATE',$3::uuid,$4,1,$5,0)", id, commandID, subjectID, workType, notBefore); err != nil {
		t.Fatalf("insert due work: %v", err)
	}
}
