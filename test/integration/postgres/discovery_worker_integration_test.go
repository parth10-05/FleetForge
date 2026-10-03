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

func TestDiscoveryWorkerAtomicallyCompletesClaimAndPersistsEvidence(t *testing.T) {
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
	processed, err := app.RunDiscoveryOnce(ctx, adapter, fixedDiscoveryExecutor{result: app.DiscoveryResult{ObservationID: "00000000-0000-0000-2801-000000000004", ObservedAt: now.Add(time.Second), Normalized: []byte(`{"reachable":true}`), Success: true}}, adapter, app.DiscoveryClaimRequest{Owner: "worker", LeaseToken: "00000000-0000-0000-2801-000000000005", Now: now, LeaseExpiresAt: now.Add(30 * time.Second)}, retryOneSecond)
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

func TestDiscoveryClaimsOldestDueWorkAndSkipsExistingLease(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const firstTargetID = "00000000-0000-0000-2806-000000000001"
	const secondTargetID = "00000000-0000-0000-2806-000000000002"
	const firstWorkID = "00000000-0000-0000-2806-000000000003"
	const secondWorkID = "00000000-0000-0000-2806-000000000004"
	for _, target := range []struct{ id, identity string }{
		{id: firstTargetID, identity: "ordered-discovery-first"},
		{id: secondTargetID, identity: "ordered-discovery-second"},
	} {
		if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ($1,$2,'endpoint','site','rack','ACTIVE')", target.id, target.identity); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 30, 0, 5, 0, 0, time.UTC)
	for _, request := range []persistence.DiscoveryScheduleRequest{
		{WorkID: firstWorkID, CommandID: "00000000-0000-0000-2806-000000000005", TargetID: firstTargetID, TargetRevision: 0, NotBefore: now},
		{WorkID: secondWorkID, CommandID: "00000000-0000-0000-2806-000000000006", TargetID: secondTargetID, TargetRevision: 0, NotBefore: now.Add(time.Second)},
	} {
		if scheduled, err := persistence.ScheduleDiscovery(ctx, pool, request); err != nil || !scheduled {
			t.Fatalf("schedule=%v err=%v", scheduled, err)
		}
	}
	adapter := persistence.DiscoveryWorkAdapter{Pool: pool}
	first, err := adapter.ClaimDiscoveryWork(ctx, app.DiscoveryClaimRequest{Owner: "first-worker", LeaseToken: "00000000-0000-0000-2806-000000000007", Now: now.Add(2 * time.Second), LeaseExpiresAt: now.Add(32 * time.Second)})
	if err != nil || first == nil || first.ID != firstWorkID {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	second, err := adapter.ClaimDiscoveryWork(ctx, app.DiscoveryClaimRequest{Owner: "second-worker", LeaseToken: "00000000-0000-0000-2806-000000000008", Now: now.Add(2 * time.Second), LeaseExpiresAt: now.Add(32 * time.Second)})
	if err != nil || second == nil || second.ID != secondWorkID {
		t.Fatalf("second claim=%+v err=%v", second, err)
	}
	if first.LeaseToken == second.LeaseToken || first.TargetID == second.TargetID {
		t.Fatalf("claim isolation violated: first=%+v second=%+v", first, second)
	}
}

func TestDiscoveryResultRejectsStaleLeaseWithoutPersistingEvidence(t *testing.T) {
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
	if claimed.Attempt != 1 {
		t.Fatalf("first claim attempt=%d", claimed.Attempt)
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

func TestDiscoveryRetryAtomicallyPersistsFailureAndReleasesLease(t *testing.T) {
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
	processed, err := app.RunDiscoveryOnce(ctx, adapter, retryingDiscoveryExecutor{result: app.DiscoveryResult{ObservationID: "00000000-0000-0000-2803-000000000004", ObservedAt: now, Normalized: []byte(`{"error_kind":"timeout"}`)}}, adapter, app.DiscoveryClaimRequest{Owner: "worker", LeaseToken: "00000000-0000-0000-2803-000000000005", Now: now, LeaseExpiresAt: now.Add(30 * time.Second)}, retryOneSecond)
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

func TestDiscoveryStopsAfterThreeRetryableAttempts(t *testing.T) {
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
		if claimed.Attempt != attempt {
			t.Fatalf("attempt=%d claimed attempt=%d", attempt, claimed.Attempt)
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

func TestDiscoveryLeaseRepairCompletesExhaustedAttemptBudget(t *testing.T) {
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

func retryOneSecond(_ app.DiscoveryWork, now time.Time) time.Time {
	return now.Add(time.Second)
}
