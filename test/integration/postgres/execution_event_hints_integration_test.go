//go:build postgresintegration

package postgres

import (
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/eventhint"
	"github.com/parth10-05/fleetforge/internal/application/execution"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestEventHintsAreDurableDeduplicatedAndOrderFenced(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	mustExecute(t, pool, ctx, "UPDATE targets SET target_key='event-target' WHERE id='"+fixture.targetID+"'")

	request := executionAdmissionRequest(fixture, "5120")
	if err := persistence.AdmitOperation(ctx, pool, request); err != nil {
		t.Fatal(err)
	}
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	submit := claimExecutionSubmitWork(t, ctx, adapter, "00000000-0000-4000-8000-000000000520")
	if err := adapter.CommitSubmitResult(ctx, submit, execution.SubmitResult{
		Class:               execution.SubmitAccepted,
		RemoteTaskReference: "/redfish/v1/TaskService/Tasks/task-event-1",
		ResponseFingerprint: executionDigest("e"),
		FinishedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	hint := eventhint.Hint{
		Source:        eventhint.Source,
		EventID:       "event-2",
		TargetKey:     "event-target",
		TaskID:        "task-event-1",
		Kind:          "TaskCompleted",
		Sequence:      2,
		ObservedAt:    now.Add(-time.Second),
		ReceivedAt:    now,
		PayloadDigest: executionDigest("f"),
	}
	if disposition, err := persistence.RecordEventHint(ctx, pool, hint); err != nil || disposition != "SCHEDULED" {
		t.Fatalf("scheduled disposition=%q err=%v", disposition, err)
	}
	if disposition, err := persistence.RecordEventHint(ctx, pool, hint); err != nil || disposition != "DUPLICATE" {
		t.Fatalf("duplicate disposition=%q err=%v", disposition, err)
	}

	late := hint
	late.EventID, late.Sequence = "event-1", 1
	late.PayloadDigest = executionDigest("a")
	if disposition, err := persistence.RecordEventHint(ctx, pool, late); err != nil || disposition != "LATE" {
		t.Fatalf("late disposition=%q err=%v", disposition, err)
	}

	var receipts, sequence, scheduledWork int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM execution_event_receipts").Scan(&receipts); err != nil || receipts != 2 {
		t.Fatalf("receipts=%d err=%v, want scheduled plus late", receipts, err)
	}
	if err := pool.QueryRow(ctx, "SELECT sequence FROM execution_event_heads WHERE target_key='event-target'").Scan(&sequence); err != nil || sequence != 2 {
		t.Fatalf("head sequence=%d err=%v, want 2", sequence, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE subject_id=$1::uuid AND work_type='TASK_OBSERVE' AND delivery_state='PENDING'", fixture.targetUpdateID).Scan(&scheduledWork); err != nil || scheduledWork < 1 {
		t.Fatalf("scheduled observation work=%d err=%v, want at least one", scheduledWork, err)
	}

	if err := persistence.QuarantineEventHint(ctx, pool, []byte("bad callback"), "REJECTED", now); err != nil {
		t.Fatal(err)
	}
	if err := persistence.QuarantineEventHint(ctx, pool, []byte("bad callback"), "REJECTED", now); err != nil {
		t.Fatal(err)
	}
	var quarantines int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM execution_event_quarantine").Scan(&quarantines); err != nil || quarantines != 1 {
		t.Fatalf("quarantines=%d err=%v, want one redacted digest record", quarantines, err)
	}
}

func TestEarlyEventHintRemainsUnmatchedWithoutCreatingAuthority(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	mustExecute(t, pool, ctx, "UPDATE targets SET target_key='early-event-target' WHERE id='"+fixture.targetID+"'")

	now := time.Now().UTC()
	hint := eventhint.Hint{
		Source:        eventhint.Source,
		EventID:       "early-event-1",
		TargetKey:     "early-event-target",
		TaskID:        "task-not-yet-accepted",
		Kind:          "TaskCompleted",
		Sequence:      1,
		ObservedAt:    now.Add(-time.Second),
		ReceivedAt:    now,
		PayloadDigest: executionDigest("b"),
	}
	if disposition, err := persistence.RecordEventHint(ctx, pool, hint); err != nil || disposition != "UNMATCHED" {
		t.Fatalf("early disposition=%q err=%v, want UNMATCHED", disposition, err)
	}
	var heads, taskWork int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM execution_event_heads WHERE target_key='early-event-target'").Scan(&heads); err != nil || heads != 0 {
		t.Fatalf("early event heads=%d err=%v, want none", heads, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE work_type='TASK_OBSERVE'").Scan(&taskWork); err != nil || taskWork != 0 {
		t.Fatalf("early task work=%d err=%v, want none", taskWork, err)
	}
}
