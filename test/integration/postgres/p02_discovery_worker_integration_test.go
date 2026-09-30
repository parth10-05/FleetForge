//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/inventory"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestP02DiscoveryWorkerAtomicallyCompletesClaimAndPersistsEvidence(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const targetID = "00000000-0000-0000-2801-000000000001"
	const workID = "00000000-0000-0000-2801-000000000002"
	if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ($1,'worker-target','endpoint','site','rack','ACTIVE')", targetID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	if ok, err := persistence.ScheduleDiscovery(ctx, pool, persistence.DiscoveryScheduleRequest{WorkID: workID, CommandID: "00000000-0000-0000-2801-000000000003", TargetID: targetID, TargetRevision: 0, NotBefore: now}); err != nil || !ok {
		t.Fatalf("schedule discovery=%v err=%v", ok, err)
	}
	adapter := persistence.DiscoveryWorkAdapter{Pool: pool}
	processed, err := app.RunDiscoveryOnce(ctx, adapter, fixedDiscoveryExecutor{result: app.DiscoveryResult{ObservationID: "00000000-0000-0000-2801-000000000004", ObservedAt: now.Add(time.Second), Normalized: []byte(`{"reachable":true}`), Success: true}}, adapter, app.DiscoveryClaimRequest{Owner: "worker", LeaseToken: "00000000-0000-0000-2801-000000000005", Now: now, LeaseExpiresAt: now.Add(30 * time.Second)}, now.Add(time.Second))
	if err != nil || !processed {
		t.Fatalf("run discovery processed=%v err=%v", processed, err)
	}
	var state, latest, good string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1", workID).Scan(&state); err != nil || state != "COMPLETED" {
		t.Fatalf("work state=%q err=%v", state, err)
	}
	if err := pool.QueryRow(ctx, "SELECT latest_attempt_id,last_good_id FROM target_discovery_heads WHERE target_id=$1", targetID).Scan(&latest, &good); err != nil || latest != "00000000-0000-0000-2801-000000000004" || good != latest {
		t.Fatalf("heads latest=%q good=%q err=%v", latest, good, err)
	}
}

func TestP02DiscoveryResultRejectsStaleLeaseWithoutPersistingEvidence(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const targetID = "00000000-0000-0000-2802-000000000001"
	const workID = "00000000-0000-0000-2802-000000000002"
	if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ($1,'stale-worker-target','endpoint','site','rack','ACTIVE')", targetID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 0, 1, 0, 0, time.UTC)
	if ok, err := persistence.ScheduleDiscovery(ctx, pool, persistence.DiscoveryScheduleRequest{WorkID: workID, CommandID: "00000000-0000-0000-2802-000000000003", TargetID: targetID, TargetRevision: 0, NotBefore: now}); err != nil || !ok {
		t.Fatalf("schedule discovery=%v err=%v", ok, err)
	}
	adapter := persistence.DiscoveryWorkAdapter{Pool: pool}
	claimed, err := adapter.ClaimDiscoveryWork(ctx, app.DiscoveryClaimRequest{Owner: "worker", LeaseToken: "00000000-0000-0000-2802-000000000004", Now: now, LeaseExpiresAt: now.Add(30 * time.Second)})
	if err != nil || claimed == nil {
		t.Fatalf("claim=%+v err=%v", claimed, err)
	}
	claimed.LeaseToken = "00000000-0000-0000-2802-000000000005"
	err = adapter.RecordDiscoveryResult(ctx, *claimed, app.DiscoveryResult{ObservationID: "00000000-0000-0000-2802-000000000006", ObservedAt: now.Add(time.Second), Normalized: []byte(`{"reachable":true}`), Success: true})
	if !errors.Is(err, persistence.ErrLeaseConflict) {
		t.Fatalf("stale result error=%v", err)
	}
	var observations, heads int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM device_observations WHERE target_id=$1", targetID).Scan(&observations); err != nil || observations != 0 {
		t.Fatalf("observations=%d err=%v", observations, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM target_discovery_heads WHERE target_id=$1", targetID).Scan(&heads); err != nil || heads != 0 {
		t.Fatalf("heads=%d err=%v", heads, err)
	}
}

func TestP02DiscoveryRetryAtomicallyPersistsFailureAndReleasesLease(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const targetID = "00000000-0000-0000-2803-000000000001"
	const workID = "00000000-0000-0000-2803-000000000002"
	if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ($1,'retry-worker-target','endpoint','site','rack','ACTIVE')", targetID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 0, 2, 0, 0, time.UTC)
	if ok, err := persistence.ScheduleDiscovery(ctx, pool, persistence.DiscoveryScheduleRequest{WorkID: workID, CommandID: "00000000-0000-0000-2803-000000000003", TargetID: targetID, TargetRevision: 0, NotBefore: now}); err != nil || !ok {
		t.Fatalf("schedule discovery=%v err=%v", ok, err)
	}
	adapter := persistence.DiscoveryWorkAdapter{Pool: pool}
	processed, err := app.RunDiscoveryOnce(ctx, adapter, retryingDiscoveryExecutor{result: app.DiscoveryResult{ObservationID: "00000000-0000-0000-2803-000000000004", ObservedAt: now, Normalized: []byte(`{"error_kind":"timeout"}`)}}, adapter, app.DiscoveryClaimRequest{Owner: "worker", LeaseToken: "00000000-0000-0000-2803-000000000005", Now: now, LeaseExpiresAt: now.Add(30 * time.Second)}, now.Add(time.Second))
	if err != nil || !processed {
		t.Fatalf("run discovery retry processed=%v err=%v", processed, err)
	}
	var state, latest string
	var lastGoodMissing bool
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1", workID).Scan(&state); err != nil || state != "PENDING" {
		t.Fatalf("retry work state=%q err=%v", state, err)
	}
	if err := pool.QueryRow(ctx, "SELECT latest_attempt_id::text,last_good_id IS NULL FROM target_discovery_heads WHERE target_id=$1", targetID).Scan(&latest, &lastGoodMissing); err != nil || latest != "00000000-0000-0000-2803-000000000004" || !lastGoodMissing {
		t.Fatalf("retry head latest=%q lastGoodMissing=%v err=%v", latest, lastGoodMissing, err)
	}
}

func TestP02DiscoveryStopsAfterThreeRetryableAttempts(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const targetID = "00000000-0000-0000-2804-000000000001"
	const workID = "00000000-0000-0000-2804-000000000002"
	if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ($1,'bounded-retry-target','endpoint','site','rack','ACTIVE')", targetID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 0, 3, 0, 0, time.UTC)
	if ok, err := persistence.ScheduleDiscovery(ctx, pool, persistence.DiscoveryScheduleRequest{WorkID: workID, CommandID: "00000000-0000-0000-2804-000000000003", TargetID: targetID, TargetRevision: 0, NotBefore: now}); err != nil || !ok {
		t.Fatalf("schedule discovery=%v err=%v", ok, err)
	}
	adapter := persistence.DiscoveryWorkAdapter{Pool: pool}
	for attempt := 1; attempt <= 3; attempt++ {
		token := "00000000-0000-0000-2804-00000000000" + string(rune('3'+attempt))
		claimed, err := adapter.ClaimDiscoveryWork(ctx, app.DiscoveryClaimRequest{Owner: "worker", LeaseToken: token, Now: now, LeaseExpiresAt: now.Add(30 * time.Second)})
		if err != nil || claimed == nil {
			t.Fatalf("attempt %d claim=%+v err=%v", attempt, claimed, err)
		}
		result := app.DiscoveryResult{ObservationID: "00000000-0000-0000-2804-00000000001" + string(rune('0'+attempt)), ObservedAt: now, Normalized: []byte(`{"error_kind":"timeout"}`)}
		if err := adapter.RecordDiscoveryRetry(ctx, *claimed, result, now, now.Add(time.Second)); err != nil {
			t.Fatalf("attempt %d retry=%v", attempt, err)
		}
		now = now.Add(time.Second)
	}
	var state string
	var attempts int
	if err := pool.QueryRow(ctx, "SELECT delivery_state,discovery_attempts FROM due_work WHERE id=$1", workID).Scan(&state, &attempts); err != nil || state != "COMPLETED" || attempts != 3 {
		t.Fatalf("bounded retry state=%q attempts=%d err=%v", state, attempts, err)
	}
	claimed, err := adapter.ClaimDiscoveryWork(ctx, app.DiscoveryClaimRequest{Owner: "worker", LeaseToken: "00000000-0000-0000-2804-000000000099", Now: now, LeaseExpiresAt: now.Add(time.Minute)})
	if err != nil || claimed != nil {
		t.Fatalf("fourth claim=%+v err=%v", claimed, err)
	}
}

func TestP02DiscoveryLeaseRepairCompletesExhaustedAttemptBudget(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const targetID = "00000000-0000-0000-2805-000000000001"
	const workID = "00000000-0000-0000-2805-000000000002"
	if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ($1,'exhausted-repair-target','endpoint','site','rack','ACTIVE')", targetID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 0, 4, 0, 0, time.UTC)
	if ok, err := persistence.ScheduleDiscovery(ctx, pool, persistence.DiscoveryScheduleRequest{WorkID: workID, CommandID: "00000000-0000-0000-2805-000000000003", TargetID: targetID, TargetRevision: 0, NotBefore: now}); err != nil || !ok {
		t.Fatalf("schedule discovery=%v err=%v", ok, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE due_work SET delivery_state='CLAIMED',discovery_attempts=3,lease_token='00000000-0000-0000-2805-000000000004',lease_owner='crashed-worker',lease_expires_at=$2 WHERE id=$1", workID, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	repaired, err := persistence.RepairExpiredDueWork(ctx, pool, now, 1)
	if err != nil || len(repaired) != 1 || repaired[0] != workID {
		t.Fatalf("repair=%v err=%v", repaired, err)
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1", workID).Scan(&state); err != nil || state != "COMPLETED" {
		t.Fatalf("repaired discovery state=%q err=%v", state, err)
	}
}

type fixedDiscoveryExecutor struct{ result app.DiscoveryResult }

func (e fixedDiscoveryExecutor) Discover(context.Context, app.DiscoveryWork) (app.DiscoveryResult, error) {
	return e.result, nil
}

type retryingDiscoveryExecutor struct{ result app.DiscoveryResult }

func (e retryingDiscoveryExecutor) Discover(context.Context, app.DiscoveryWork) (app.DiscoveryResult, error) {
	return e.result, errors.New("timeout")
}
