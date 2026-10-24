//go:build postgresintegration

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestAlreadyCurrentPrecheckCompletesFencedWorkWithoutAdmission(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	if _, err := pool.Exec(ctx, "UPDATE target_updates SET state='PRECHECK',revision=1,command_epoch=7 WHERE id=$1::uuid", fixture.targetUpdateID); err != nil {
		t.Fatal(err)
	}
	const workID = "00000000-0000-0000-5201-000000000001"
	const commandID = "00000000-0000-0000-5201-000000000002"
	if _, err := pool.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES ($1::uuid,$2::uuid,'TARGET_UPDATE',$3::uuid,'PRECHECK',1,$4,1,7)`, workID, commandID, fixture.targetUpdateID, time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return now }}
	work, claimed, err := adapter.ClaimPrecheckWork(ctx, "precheck-test", "00000000-0000-0000-5201-000000000003")
	if err != nil || !claimed {
		t.Fatalf("work=%+v claimed=%t err=%v", work, claimed, err)
	}
	snapshot, err := adapter.LoadPrecheck(ctx, work)
	if err != nil {
		t.Fatal(err)
	}
	completion := execution.AlreadyCurrentCommand{
		Work: work, Snapshot: snapshot,
		Evidence: execution.PrecheckEvidence{CurrentVersion: snapshot.ExpectedVersion, Healthy: true, EvidenceFingerprint: executionDigest("f"), ObservedAt: now},
		Envelope: execution.CompletionEnvelope{
			CampaignTargetID: fixture.campaignTargetID, ExpectedTargetState: "PRECHECK", NextTargetState: "SUCCEEDED",
			ActorID: "system", CorrelationID: commandID, CausationID: commandID, Action: "FIRMWARE_ALREADY_CURRENT", ReasonCode: "target.verification.sufficient", AuditEventSequence: 0,
			OutboxEventID: "00000000-0000-0000-5201-000000000004", OutboxEventType: "target.firmware_already_current", OutboxSchemaVersion: 1, OutboxPayload: json.RawMessage(`{"succeeded":true}`),
			DecisionInputID: "00000000-0000-0000-5201-000000000005", DecisionSequence: 0, DecisionLogicalTime: now, ReducerVersion: "domain-v1", DecisionPolicyVersion: "policy-v1", NormalizedFacts: json.RawMessage(`{"healthy":true,"version_matches":true}`), DecisionResult: "SUCCEEDED",
		},
	}
	if err := adapter.CompleteAlreadyCurrent(ctx, completion); err != nil {
		t.Fatal(err)
	}
	var state, delivery string
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT state,revision FROM target_updates WHERE id=$1::uuid", fixture.targetUpdateID).Scan(&state, &revision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1::uuid", workID).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	if state != "SUCCEEDED" || revision != 2 || delivery != "COMPLETED" {
		t.Fatalf("target=%s/%d due_work=%s", state, revision, delivery)
	}
	for name, query := range map[string]string{
		"precheck evidence": "SELECT COUNT(*) FROM device_observations WHERE target_update_id='" + fixture.targetUpdateID + "'::uuid AND kind='FIRMWARE_PRECHECK'",
		"decision":          "SELECT COUNT(*) FROM decision_inputs WHERE aggregate_id='" + fixture.targetUpdateID + "'::uuid AND result='SUCCEEDED'",
		"audit":             "SELECT COUNT(*) FROM audit_events WHERE aggregate_id='" + fixture.targetUpdateID + "'::uuid AND action='FIRMWARE_ALREADY_CURRENT'",
		"outbox":            "SELECT COUNT(*) FROM outbox_events WHERE event_id='00000000-0000-0000-5201-000000000004'::uuid",
	} {
		var count int
		if err := pool.QueryRow(context.Background(), query).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", name, count, err)
		}
	}
	for name, query := range map[string]string{
		"firmware attempts":      "SELECT COUNT(*) FROM firmware_update_attempts WHERE target_update_id='" + fixture.targetUpdateID + "'::uuid",
		"operation reservations": "SELECT COUNT(*) FROM operation_reservations WHERE target_id='" + fixture.targetID + "'::uuid",
		"submit work":            "SELECT COUNT(*) FROM due_work WHERE subject_id='" + fixture.targetUpdateID + "'::uuid AND work_type='SUBMIT'",
	} {
		var count int
		if err := pool.QueryRow(context.Background(), query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v, want no submission side effect", name, count, err)
		}
	}
}
