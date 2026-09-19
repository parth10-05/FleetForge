//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
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
		if err != nil {
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

func insertDueWork(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id, commandID, subjectID, workType string, notBefore time.Time) {
	t.Helper()
	if _, err := pool.Exec(ctx, "INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision) VALUES ($1::uuid,$2::uuid,'TARGET_UPDATE',$3::uuid,$4,1,$5,0)", id, commandID, subjectID, workType, notBefore); err != nil {
		t.Fatalf("insert due work: %v", err)
	}
}
