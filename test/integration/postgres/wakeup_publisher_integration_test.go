//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/parth10-05/fleetforge/internal/application/orchestration"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

type wakeupPublisherFunc func(context.Context, orchestration.Envelope) (orchestration.PublishOutcome, error)

func (f wakeupPublisherFunc) Publish(ctx context.Context, envelope orchestration.Envelope) (orchestration.PublishOutcome, error) {
	return f(ctx, envelope)
}

func TestDueWorkCreatesTypedWakeupAndConcurrentPublishersFenceIt(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 10, 5, 1, 0, 0, 0, time.UTC)
	insertDueWork(t, pool, ctx, "00000000-0000-0000-1401-000000000001", "00000000-0000-0000-1401-000000000002", fixture.targetUpdateID, "PRECHECK", now.Add(-time.Minute))

	type result struct {
		rows []persistence.ClaimedWakeup
		err  error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i, token := range []string{"00000000-0000-0000-1401-000000000003", "00000000-0000-0000-1401-000000000004"} {
		group.Add(1)
		go func(owner, claimToken string) {
			defer group.Done()
			<-start
			rows, err := persistence.ClaimWorkWakeups(context.Background(), pool, persistence.ClaimOutboxRequest{Owner: owner, Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{claimToken}})
			results <- result{rows, err}
		}("publisher-"+string(rune('a'+i)), token)
	}
	close(start)
	group.Wait()
	close(results)
	var claimed persistence.ClaimedWakeup
	count := 0
	for got := range results {
		if got.err != nil && !errors.Is(got.err, persistence.ErrOutboxClaimConflict) {
			t.Fatalf("claim: %v", got.err)
		}
		for _, row := range got.rows {
			claimed, count = row, count+1
		}
	}
	if count != 1 {
		t.Fatalf("claimed wakeups=%d, want 1", count)
	}
	if claimed.Envelope.CommandID != "00000000-0000-0000-1401-000000000002" || claimed.Envelope.DeliveryGeneration != 0 {
		t.Fatalf("wrong envelope: %#v", claimed.Envelope)
	}
	if err := persistence.MarkWorkWakeupPublished(ctx, pool, persistence.OutboxResult{EventID: claimed.EventID, ClaimToken: "00000000-0000-0000-1401-000000000005", Now: now}); !errors.Is(err, persistence.ErrOutboxClaimConflict) {
		t.Fatalf("stale mark=%v", err)
	}
	if err := persistence.MarkWorkWakeupPublished(ctx, pool, persistence.OutboxResult{EventID: claimed.EventID, ClaimToken: claimed.ClaimToken, Now: now}); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownWakeupPublishRetriesSameIntentAndGeneration(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 10, 5, 2, 0, 0, 0, time.UTC)
	insertDueWork(t, pool, ctx, "00000000-0000-0000-1402-000000000001", "00000000-0000-0000-1402-000000000002", fixture.targetUpdateID, "PRECHECK", now.Add(-time.Minute))
	unknown := wakeupPublisherFunc(func(context.Context, orchestration.Envelope) (orchestration.PublishOutcome, error) {
		return orchestration.PublishOutcomeUnknown, nil
	})
	if err := persistence.PublishClaimedWakeups(ctx, pool, unknown, persistence.ClaimOutboxRequest{Owner: "publisher-a", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{"00000000-0000-0000-1402-000000000003"}}, 3, func(time.Time) time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	second, err := persistence.ClaimWorkWakeups(ctx, pool, persistence.ClaimOutboxRequest{Owner: "publisher-b", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{"00000000-0000-0000-1402-000000000004"}})
	if err != nil || len(second) != 1 {
		t.Fatalf("reclaim=%v %v", second, err)
	}
	if second[0].Envelope.CommandID != "00000000-0000-0000-1402-000000000002" || second[0].Envelope.DeliveryGeneration != 0 {
		t.Fatalf("identity changed: %#v", second[0].Envelope)
	}
}

func TestPermanentWakeupRejectionCreatesDurableQuarantine(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)
	insertDueWork(t, pool, ctx, "00000000-0000-0000-1403-000000000001", "00000000-0000-0000-1403-000000000002", fixture.targetUpdateID, "PRECHECK", now.Add(-time.Minute))
	permanent := wakeupPublisherFunc(func(context.Context, orchestration.Envelope) (orchestration.PublishOutcome, error) {
		return orchestration.PublishRejectedPermanent, nil
	})
	if err := persistence.PublishClaimedWakeups(ctx, pool, permanent, persistence.ClaimOutboxRequest{Owner: "publisher", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{"00000000-0000-0000-1403-000000000003"}}, 3, func(time.Time) time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	var state, reason string
	if err := pool.QueryRow(ctx, "SELECT o.delivery_state,q.reason_code FROM outbox_events o JOIN wakeup_quarantine q ON q.due_work_id=o.due_work_id WHERE o.due_work_id=$1::uuid", "00000000-0000-0000-1403-000000000001").Scan(&state, &reason); err != nil || state != "QUARANTINED" || reason != "PERMANENT_PUBLISH_REJECTION" {
		t.Fatalf("state=%q reason=%q err=%v", state, reason, err)
	}
}

func TestWakeupCrashAfterPublishIsReclaimedWithSameIdentity(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC)
	insertDueWork(t, pool, ctx, "00000000-0000-0000-1405-000000000001", "00000000-0000-0000-1405-000000000002", fixture.targetUpdateID, "PRECHECK", now.Add(-time.Minute))
	first, err := persistence.ClaimWorkWakeups(ctx, pool, persistence.ClaimOutboxRequest{Owner: "publisher-a", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{"00000000-0000-0000-1405-000000000003"}})
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim=%v err=%v", first, err)
	}
	// The external queue would have accepted this envelope here; the simulated
	// process dies before its fenced mark-published transaction.
	if _, err := persistence.RepairExpiredOutbox(ctx, pool, now.Add(2*time.Minute), 1); err != nil {
		t.Fatal(err)
	}
	second, err := persistence.ClaimWorkWakeups(ctx, pool, persistence.ClaimOutboxRequest{Owner: "publisher-b", Now: now.Add(2 * time.Minute), LeaseExpiresAt: now.Add(3 * time.Minute), Tokens: []string{"00000000-0000-0000-1405-000000000004"}})
	if err != nil || len(second) != 1 {
		t.Fatalf("reclaim=%v err=%v", second, err)
	}
	if second[0].EventID != first[0].EventID || second[0].Envelope.CommandID != first[0].Envelope.CommandID || second[0].Envelope.DeliveryGeneration != first[0].Envelope.DeliveryGeneration {
		t.Fatalf("crash recovery changed identity: first=%#v second=%#v", first[0], second[0])
	}
}

func TestOutboxRetryUsesExactAttemptBudget(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 10, 5, 5, 0, 0, 0, time.UTC)
	const eventID = "00000000-0000-0000-1406-000000000001"
	insertOutbox(t, pool, ctx, eventID, now.Add(-time.Minute))
	for attempt, token := range []string{"00000000-0000-0000-1406-000000000002", "00000000-0000-0000-1406-000000000003", "00000000-0000-0000-1406-000000000004"} {
		claimed, err := persistence.ClaimOutbox(ctx, pool, persistence.ClaimOutboxRequest{Owner: "publisher", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{token}})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("attempt %d claim=%v err=%v", attempt+1, claimed, err)
		}
		if err := persistence.RetryOrQuarantineOutbox(ctx, pool, persistence.OutboxResult{EventID: eventID, ClaimToken: token, Now: now, RetryAt: now, ErrorClass: "PUBLISH_FAILED", MaxAttempts: 3}); err != nil {
			t.Fatalf("attempt %d retry: %v", attempt+1, err)
		}
		var state string
		if err := pool.QueryRow(ctx, "SELECT delivery_state FROM outbox_events WHERE event_id=$1", eventID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		want := "PENDING"
		if attempt == 2 {
			want = "QUARANTINED"
		}
		if state != want {
			t.Fatalf("attempt %d state=%q want %q", attempt+1, state, want)
		}
	}
}

func TestWakeupLinkageConstraintsRejectPartialRows(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	_, err := pool.Exec(ctx, "INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload,delivery_kind,due_work_id) VALUES ('00000000-0000-0000-1404-000000000001','CAMPAIGN','00000000-0000-0000-1404-000000000002',0,'work.wakeup.requested',1,'{}','WORK_WAKEUP','00000000-0000-0000-1404-000000000003')")
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) || databaseError.Code != "23514" {
		t.Fatalf("partial linkage error=%v", err)
	}
}

func TestWakeupSpecificClaimRejectsStaleFencingAndCoownership(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	const dueWorkID = "00000000-0000-0000-1407-000000000001"
	const commandID = "00000000-0000-0000-1407-000000000002"
	insertDueWork(t, pool, ctx, dueWorkID, commandID, fixture.targetUpdateID, "PRECHECK", now.Add(-time.Minute))
	request := persistence.ClaimWakeupWorkRequest{Owner: "worker-a", LeaseToken: "00000000-0000-0000-1407-000000000003", DueWorkID: dueWorkID, CommandID: commandID, DeliveryGeneration: 0, ExpectedRevision: 0, Now: now, LeaseExpiresAt: now.Add(time.Minute)}
	claimed, ok, err := persistence.ClaimDueWorkForWakeup(ctx, pool, request)
	if err != nil || !ok {
		t.Fatalf("claim=%#v ok=%t err=%v", claimed, ok, err)
	}
	if claimed.LeaseToken != request.LeaseToken {
		t.Fatalf("token=%q", claimed.LeaseToken)
	}
	request.Owner, request.LeaseToken = "worker-b", "00000000-0000-0000-1407-000000000004"
	if _, ok, err := persistence.ClaimDueWorkForWakeup(ctx, pool, request); err != nil || ok {
		t.Fatalf("co-owner ok=%t err=%v", ok, err)
	}
	request.DeliveryGeneration = 1
	if _, ok, err := persistence.ClaimDueWorkForWakeup(ctx, pool, request); err != nil || ok {
		t.Fatalf("stale generation ok=%t err=%v", ok, err)
	}
}

func TestWakeupAdapterCompletesWithDurableEvidenceAndFencesReplay(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC)
	const dueWorkID = "00000000-0000-0000-1408-000000000001"
	const commandID = "00000000-0000-0000-1408-000000000002"
	insertDueWork(t, pool, ctx, dueWorkID, commandID, fixture.targetUpdateID, "SAFETY_PROBE", now.Add(-time.Minute))
	adapter := persistence.WakeupWorkAdapter{Pool: pool, Owner: "worker", LeaseDuration: time.Minute, Now: func() time.Time { return now }, NewToken: func() (string, error) { return "00000000-0000-0000-1408-000000000003", nil }}
	envelope := orchestration.Envelope{SchemaVersion: orchestration.WakeupSchemaV1, QueueClass: orchestration.QueueControl, DueWorkID: dueWorkID, CommandID: commandID, DeliveryGeneration: 0, WorkType: "SAFETY_PROBE", SubjectType: "TARGET_UPDATE", SubjectID: fixture.targetUpdateID, ExpectedRevision: 0, NotBefore: now.Add(-time.Minute)}
	work, disposition, err := adapter.Claim(ctx, envelope)
	if err != nil || disposition != orchestration.ClaimOwned {
		t.Fatalf("claim=%#v disposition=%v err=%v", work, disposition, err)
	}
	if err := adapter.Complete(ctx, work); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Complete(ctx, work); !errors.Is(err, persistence.ErrLeaseConflict) {
		t.Fatalf("replay completion=%v", err)
	}
	var state string
	var evidence int
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1::uuid", dueWorkID).Scan(&state); err != nil || state != "COMPLETED" {
		t.Fatalf("state=%q err=%v", state, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM wakeup_execution_evidence WHERE due_work_id=$1::uuid", dueWorkID).Scan(&evidence); err != nil || evidence != 1 {
		t.Fatalf("evidence=%d err=%v", evidence, err)
	}
}

func TestWakeupAdapterQuarantineIsDurablyDeduplicated(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	adapter := persistence.WakeupWorkAdapter{Pool: pool, Now: func() time.Time { return now }}
	receipt := orchestration.Receipt{Raw: []byte("not-json")}
	if err := adapter.Quarantine(ctx, receipt, "MALFORMED_ENCODING"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Quarantine(ctx, receipt, "MALFORMED_ENCODING"); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := pool.QueryRow(ctx, "SELECT occurrences FROM wakeup_quarantine WHERE reason_code='MALFORMED_ENCODING'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("occurrences=%d err=%v", count, err)
	}
}

func TestRepairMissingWakeupsAdvancesGenerationWithoutChangingCommand(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	const dueWorkID = "00000000-0000-0000-1409-000000000001"
	const commandID = "00000000-0000-0000-1409-000000000002"
	insertDueWork(t, pool, ctx, dueWorkID, commandID, fixture.targetUpdateID, "SAFETY_PROBE", now.Add(-time.Hour))
	if _, err := pool.Exec(ctx, "UPDATE outbox_events SET delivery_state='PUBLISHED',published_at=$2 WHERE due_work_id=$1::uuid", dueWorkID, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	repaired, err := persistence.RepairMissingWakeups(ctx, pool, persistence.RepairWakeupRequest{Now: now, DeliveryGrace: time.Minute, Limit: 1, MaxGeneration: 3})
	if err != nil || len(repaired) != 1 || repaired[0] != dueWorkID {
		t.Fatalf("repair=%v err=%v", repaired, err)
	}
	var generation int64
	var gotCommand string
	if err := pool.QueryRow(ctx, "SELECT delivery_generation,command_id::text FROM due_work WHERE id=$1::uuid", dueWorkID).Scan(&generation, &gotCommand); err != nil || generation != 1 || gotCommand != commandID {
		t.Fatalf("generation=%d command=%s err=%v", generation, gotCommand, err)
	}
	var intents int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE due_work_id=$1::uuid AND delivery_generation=1 AND delivery_kind='WORK_WAKEUP'", dueWorkID).Scan(&intents); err != nil || intents != 1 {
		t.Fatalf("new intents=%d err=%v", intents, err)
	}
}

func TestRepairMissingWakeupsSkipsLiveClaimsAndGenerationCap(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	now := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	const liveID = "00000000-0000-0000-1410-000000000001"
	const cappedID = "00000000-0000-0000-1410-000000000002"
	insertDueWork(t, pool, ctx, liveID, "00000000-0000-0000-1410-000000000003", fixture.targetUpdateID, "SAFETY_PROBE", now.Add(-time.Hour))
	insertDueWork(t, pool, ctx, cappedID, "00000000-0000-0000-1410-000000000004", fixture.targetUpdateID, "SAFETY_PROBE", now.Add(-time.Hour))
	if _, err := pool.Exec(ctx, "UPDATE outbox_events SET delivery_state='CLAIMED',claim_token='00000000-0000-0000-1410-000000000005',claim_owner='live',claim_expires_at=$2 WHERE due_work_id=$1::uuid", liveID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE outbox_events SET delivery_state='PUBLISHED',published_at=$2 WHERE due_work_id=$1::uuid", cappedID, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE due_work SET delivery_generation=3 WHERE id=$1::uuid", cappedID); err != nil {
		t.Fatal(err)
	}
	repaired, err := persistence.RepairMissingWakeups(ctx, pool, persistence.RepairWakeupRequest{Now: now, DeliveryGrace: time.Minute, Limit: 10, MaxGeneration: 3})
	if err != nil || len(repaired) != 0 {
		t.Fatalf("repair=%v err=%v", repaired, err)
	}
	for _, id := range []string{liveID, cappedID} {
		var generation int64
		if err := pool.QueryRow(ctx, "SELECT delivery_generation FROM due_work WHERE id=$1::uuid", id).Scan(&generation); err != nil {
			t.Fatal(err)
		}
		if id == liveID && generation != 0 {
			t.Fatalf("live generation=%d", generation)
		}
		if id == cappedID && generation != 3 {
			t.Fatalf("capped generation=%d", generation)
		}
	}
}

func TestCapacityAvailableReadsExistingReservationTruthWithoutAdmission(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	for _, scope := range []persistence.ScopeCapacity{{Level: "GLOBAL", Key: "GLOBAL", Limit: 1}, {Level: "SITE", Key: "site-a", Limit: 1}, {Level: "RACK", Key: "rack-a", Limit: 1}} {
		if _, err := pool.Exec(ctx, "INSERT INTO scope_capacities (scope_level,scope_key,capacity_limit) VALUES ($1,$2,$3)", scope.Level, scope.Key, scope.Limit); err != nil {
			t.Fatal(err)
		}
	}
	global, site, rack := persistence.ScopeCapacity{Level: "GLOBAL", Key: "GLOBAL", Limit: 1}, persistence.ScopeCapacity{Level: "SITE", Key: "site-a", Limit: 1}, persistence.ScopeCapacity{Level: "RACK", Key: "rack-a", Limit: 1}
	available, err := persistence.CapacityAvailable(ctx, pool, global, site, rack)
	if err != nil || !available {
		t.Fatalf("available=%t err=%v", available, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE scope_capacities SET active_reservation_count=1 WHERE scope_level='RACK' AND scope_key='rack-a'"); err != nil {
		t.Fatal(err)
	}
	available, err = persistence.CapacityAvailable(ctx, pool, global, site, rack)
	if err != nil || available {
		t.Fatalf("exhausted available=%t err=%v", available, err)
	}
	var admissions, reservations int
	if err := pool.QueryRow(ctx, "SELECT (SELECT COUNT(*) FROM submission_admissions),(SELECT COUNT(*) FROM operation_reservations)").Scan(&admissions, &reservations); err != nil || admissions != 0 || reservations != 0 {
		t.Fatalf("admissions=%d reservations=%d err=%v", admissions, reservations, err)
	}
}
