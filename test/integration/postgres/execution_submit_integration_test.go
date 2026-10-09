//go:build postgresintegration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestFencedSubmitResultAcceptedSchedulesObservationExactlyOnce(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	req := executionAdmissionRequest(fixture, "5101")
	if err := persistence.AdmitOperation(ctx, pool, req); err != nil {
		t.Fatal(err)
	}
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	work := claimExecutionSubmitWork(t, ctx, adapter, "00000000-0000-0000-5101-000000000099")
	attempt, err := adapter.LoadAttempt(ctx, work)
	if err != nil || attempt.AttemptID != req.FirmwareAttempt.AttemptID {
		t.Fatalf("attempt=%+v err=%v", attempt, err)
	}
	finished := time.Now().UTC().Add(time.Second)
	if err := adapter.CommitSubmitResult(ctx, work, execution.SubmitResult{Class: execution.SubmitAccepted, RemoteTaskReference: "/redfish/v1/TaskService/Tasks/task-1", ResponseFingerprint: executionDigest("c"), FinishedAt: finished}); err != nil {
		t.Fatal(err)
	}
	var status, state, delivery, nextType string
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT a.status,tu.state,tu.revision,d.delivery_state,d.work_type
FROM firmware_update_attempts a JOIN target_updates tu ON tu.id=a.target_update_id
JOIN due_work d ON d.subject_id=tu.id AND d.work_type='TASK_OBSERVE'
WHERE a.id=$1::uuid`, attempt.AttemptID).Scan(&status, &state, &revision, &delivery, &nextType); err != nil {
		t.Fatal(err)
	}
	if status != "ACCEPTED" || state != "UPDATE_ACCEPTED" || revision != 2 || delivery != "PENDING" || nextType != "TASK_OBSERVE" {
		t.Fatalf("status=%s state=%s revision=%d work=%s/%s", status, state, revision, delivery, nextType)
	}
	var submits int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE work_type='SUBMIT'").Scan(&submits); err != nil || submits != 1 {
		t.Fatalf("submit work count=%d err=%v, want original only", submits, err)
	}
}

func TestFencedAmbiguousSubmitSchedulesReconciliationWithoutSecondSubmit(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	req := executionAdmissionRequest(fixture, "5102")
	if err := persistence.AdmitOperation(ctx, pool, req); err != nil {
		t.Fatal(err)
	}
	adapter := persistence.ExecutionAdapter{Pool: pool}
	work := claimExecutionSubmitWork(t, ctx, adapter, "00000000-0000-0000-5102-000000000099")
	if err := adapter.CommitSubmitResult(ctx, work, execution.SubmitResult{Class: execution.SubmitPossiblyAccepted, ResponseFingerprint: executionDigest("d"), FinishedAt: time.Now().UTC().Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	var status, state, delivery, nextType string
	var resume *string
	if err := pool.QueryRow(ctx, `SELECT a.status,tu.state,tu.resume_step,d.delivery_state,d.work_type
FROM firmware_update_attempts a JOIN target_updates tu ON tu.id=a.target_update_id
JOIN due_work d ON d.subject_id=tu.id AND d.work_type='AMBIGUITY_RECONCILE'
WHERE a.id=$1::uuid`, req.FirmwareAttempt.AttemptID).Scan(&status, &state, &resume, &delivery, &nextType); err != nil {
		t.Fatal(err)
	}
	if status != "AMBIGUOUS" || state != "NEEDS_ATTENTION" || resume != nil || delivery != "PENDING" || nextType != "AMBIGUITY_RECONCILE" {
		t.Fatalf("status=%s state=%s resume=%v work=%s/%s", status, state, resume, delivery, nextType)
	}
	var submits int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE work_type='SUBMIT'").Scan(&submits); err != nil || submits != 1 {
		t.Fatalf("submit work count=%d err=%v, want original only", submits, err)
	}
}

func claimExecutionSubmitWork(t *testing.T, ctx context.Context, adapter persistence.ExecutionAdapter, token string) execution.WorkFence {
	t.Helper()
	work, claimed, err := adapter.ClaimSubmitWork(ctx, "execution-test", token)
	if err != nil || !claimed {
		t.Fatalf("claim=%+v claimed=%t err=%v", work, claimed, err)
	}
	return work
}

func executionAdmissionRequest(f ownershipFixture, suffix string) persistence.AdmissionRequest {
	req := admissionRequest(f, suffix)
	base := "00000000-0000-0000-" + suffix + "-0000000000"
	req.FirmwareAttempt = &persistence.FirmwareAttemptRequest{AttemptID: base + "10", Provider: "local-versioned-v1", Container: "firmware", ObjectKey: "verified/firmware.bin", ObjectVersion: "v1", ObjectSize: 4096, SHA256Digest: executionDigest("a"), ExpectedVersion: "1.2.3", RequestFingerprint: executionDigest("b")}
	return req
}

func executionDigest(character string) string {
	return "sha256:" + string([]byte(character)[0]) + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}
