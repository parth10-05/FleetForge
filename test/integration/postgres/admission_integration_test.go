//go:build postgresintegration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestAdmissionPersistsFencedIntentReservationsAndEvidenceAtomically(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)

	if err := persistence.AdmitOperation(ctx, pool, admissionRequest(fixture, "1001")); err != nil {
		t.Fatalf("AdmitOperation: %v", err)
	}
	var state string
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT state, revision FROM target_updates WHERE id=$1", fixture.targetUpdateID).Scan(&state, &revision); err != nil {
		t.Fatalf("read target update: %v", err)
	}
	if state != "SUBMITTING" || revision != 1 {
		t.Fatalf("target update = %s/%d, want SUBMITTING/1", state, revision)
	}
	for _, query := range []string{
		"SELECT COUNT(*) FROM submission_admissions",
		"SELECT COUNT(*) FROM operation_reservations WHERE state='ACTIVE'",
		"SELECT COUNT(*) FROM step_attempts",
		"SELECT COUNT(*) FROM decision_inputs",
		"SELECT COUNT(*) FROM audit_events",
		"SELECT COUNT(*) FROM outbox_events",
		"SELECT COUNT(*) FROM due_work",
	} {
		var count int
		if err := pool.QueryRow(ctx, query).Scan(&count); err != nil || count != 1 && query != "SELECT COUNT(*) FROM operation_reservations WHERE state='ACTIVE'" && query != "SELECT COUNT(*) FROM outbox_events" {
			t.Fatalf("%s count=%d err=%v, want 1", query, count, err)
		}
		if query == "SELECT COUNT(*) FROM operation_reservations WHERE state='ACTIVE'" && count != 3 {
			t.Fatalf("active reservation count=%d, want 3", count)
		}
		if query == "SELECT COUNT(*) FROM outbox_events" && count != 2 {
			t.Fatalf("outbox count=%d, want domain event plus typed wakeup", count)
		}
	}
	var active int64
	if err := pool.QueryRow(ctx, "SELECT SUM(active_reservation_count) FROM scope_capacities").Scan(&active); err != nil || active != 3 {
		t.Fatalf("scope capacity sum=%d err=%v, want 3", active, err)
	}
}

func TestAdmissionPersistsInitialFirmwareAttemptAtomically(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	req := admissionRequest(fixture, "1011")
	base := "00000000-0000-0000-1011-0000000000"
	req.FirmwareAttempt = &persistence.FirmwareAttemptRequest{
		AttemptID: base + "10", Provider: "local-versioned-v1", Container: "firmware", ObjectKey: "verified/firmware.bin", ObjectVersion: "v1", ObjectSize: 4096,
		SHA256Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpectedVersion: "1.2.3", RequestFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	req.PrecheckObservedAt = time.Now().UTC()
	req.PrecheckEvidence = json.RawMessage(`{"current_version":"1.0.0","capable":true,"healthy":true,"compatible":true,"evidence_fingerprint":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}`)
	if err := persistence.AdmitOperation(ctx, pool, req); err != nil {
		t.Fatalf("AdmitOperation: %v", err)
	}
	var status, initial, active string
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT a.status,a.admitted_revision,s.initial_attempt_id::text,s.active_attempt_id::text
FROM firmware_update_attempts a JOIN submission_admissions s ON s.id=a.submission_admission_id
WHERE a.id=$1::uuid`, req.FirmwareAttempt.AttemptID).Scan(&status, &revision, &initial, &active); err != nil {
		t.Fatal(err)
	}
	if status != "ADMITTED" || revision != 1 || initial != req.FirmwareAttempt.AttemptID || active != req.FirmwareAttempt.AttemptID {
		t.Fatalf("attempt=%s revision=%d initial=%s active=%s", status, revision, initial, active)
	}
}

func TestAdmissionStaleEpochRollsBackCapacityAndEvidence(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 8)
	if _, err := pool.Exec(ctx, "UPDATE campaigns SET command_epoch=9 WHERE id=$1", fixture.campaignID); err != nil {
		t.Fatalf("fence campaign: %v", err)
	}
	err := persistence.AdmitOperation(ctx, pool, admissionRequest(fixture, "1002"))
	if !errors.Is(err, persistence.ErrAdmissionBlocked) {
		t.Fatalf("stale epoch error=%v, want ErrAdmissionBlocked", err)
	}
	assertAdmissionAbsent(t, pool, ctx)
}

// TestAdmissionBlockedByCommittedPauseForces the admission/pause boundary:
// admission waits on the campaign lock, then observes the committed epoch.
func TestAdmissionBlockedByCommittedPause(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	locker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Rollback(ctx) //nolint:errcheck
	if _, err := locker.Exec(ctx, "SELECT id FROM campaigns WHERE id=$1::uuid FOR UPDATE", fixture.campaignID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		request := admissionRequest(fixture, "1004")
		err := persistence.AdmitOperation(context.Background(), pool, request)
		// Serializable admission may abort after waiting for the pause writer.
		// Retrying the same fenced command must then observe the committed pause.
		if errors.Is(err, persistence.ErrRevisionConflict) {
			err = persistence.AdmitOperation(context.Background(), pool, request)
		}
		result <- err
	}()
	if _, err := locker.Exec(ctx, "UPDATE campaigns SET state='PAUSED', command_epoch=8 WHERE id=$1::uuid", fixture.campaignID); err != nil {
		t.Fatal(err)
	}
	if err := locker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, persistence.ErrAdmissionBlocked) {
		t.Fatalf("admission after pause=%v, want ErrAdmissionBlocked", err)
	}
	assertAdmissionAbsent(t, pool, ctx)
}

func TestAdmissionDuplicateIdentityRollsBackTentativeCapacity(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	insertAdmission(t, pool, ctx, "00000000-0000-0000-1003-000000000001", "00000000-0000-0000-1003-000000000002", fixture)
	err := persistence.AdmitOperation(ctx, pool, admissionRequest(fixture, "1003"))
	if !errors.Is(err, persistence.ErrAdmissionDuplicate) {
		t.Fatalf("duplicate error=%v, want ErrAdmissionDuplicate", err)
	}
	var capacityRows, active int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*), COALESCE(SUM(active_reservation_count),0) FROM scope_capacities").Scan(&capacityRows, &active); err != nil || capacityRows != 0 || active != 0 {
		t.Fatalf("duplicate left capacity rows=%d active=%d err=%v, want 0/0", capacityRows, active, err)
	}
}

func TestScopeCapacityConstraintsRejectInvalidAccounting(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	assertConstraint(t, pool, ctx, "INSERT INTO scope_capacities (scope_level,scope_key,capacity_limit) VALUES ('GLOBAL','wrong',1)")
	assertConstraint(t, pool, ctx, "INSERT INTO scope_capacities (scope_level,scope_key,capacity_limit) VALUES ('SITE','s',0)")
	assertConstraint(t, pool, ctx, "INSERT INTO scope_capacities (scope_level,scope_key,capacity_limit,active_reservation_count) VALUES ('SITE','s',1,2)")
}

func prepareAdmissionFixture(t *testing.T, pool *pgxpool.Pool, ctx context.Context, fixture ownershipFixture, epoch int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, "UPDATE campaigns SET state='RUNNING_WAVES', command_epoch=$2 WHERE id=$1::uuid", fixture.campaignID, epoch); err != nil {
		t.Fatalf("prepare admitting campaign: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE target_updates SET command_epoch=$2 WHERE id=$1::uuid", fixture.targetUpdateID, epoch); err != nil {
		t.Fatalf("prepare target epoch: %v", err)
	}
	insertEnrollment(t, pool, ctx, "00000000-0000-0000-1000-000000000001", fixture)
}

func admissionRequest(f ownershipFixture, suffix string) persistence.AdmissionRequest {
	base := "00000000-0000-0000-" + suffix + "-0000000000"
	return persistence.AdmissionRequest{
		CampaignID: f.campaignID, CampaignTargetID: f.campaignTargetID, TargetID: f.targetID, ArtifactID: f.artifactID, TargetUpdateID: f.targetUpdateID,
		ExpectedTargetRevision: 0, CapturedCommandEpoch: 7, ExpectedTargetState: "PENDING", NextTargetState: "SUBMITTING",
		CommandID: base + "01", Step: "SUBMISSION", AdmissionID: base + "02", StepAttemptID: base + "03",
		Global: persistence.ScopeCapacity{Level: "GLOBAL", Key: "GLOBAL", Limit: 2}, Site: persistence.ScopeCapacity{Level: "SITE", Key: "site-a", Limit: 2}, Rack: persistence.ScopeCapacity{Level: "RACK", Key: "rack-a", Limit: 2},
		ReservationIDs: [3]string{base + "04", base + "05", base + "06"}, ActorID: "system", CorrelationID: base + "01", CausationID: base + "01", Action: "SUBMISSION_ADMITTED", ReasonCode: "target.submission.authorized", AuditEventSequence: 0,
		OutboxEventID: base + "07", OutboxEventType: "target.submission_admitted", OutboxSchemaVersion: 1, OutboxPayload: json.RawMessage(`{"admitted":true}`),
		DueWorkID: base + "08", DueWorkWorkType: "SUBMIT", DueWorkSchemaVersion: 1, DueWorkNotBefore: time.Now().UTC(),
		DecisionInputID: base + "09", DecisionSequence: 0, DecisionLogicalTime: time.Now().UTC(), ReducerVersion: "domain-v1", DecisionPolicyVersion: "policy-v1", NormalizedFacts: json.RawMessage(`{"eligible":true}`), DecisionResult: "SUBMITTING",
	}
}

func assertAdmissionAbsent(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	for _, table := range []string{"submission_admissions", "operation_reservations", "step_attempts", "decision_inputs", "audit_events", "outbox_events", "due_work", "scope_capacities"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v, want 0 after rejected admission", table, count, err)
		}
	}
}
