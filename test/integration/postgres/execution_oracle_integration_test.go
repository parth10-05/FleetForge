//go:build postgresintegration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestPersistedOracleRejectsReceivedOperationClaimedProvenAbsent(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	req := executionAdmissionRequest(fixture, "5104")
	if err := persistence.AdmitOperation(ctx, pool, req); err != nil {
		t.Fatal(err)
	}
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	work := claimExecutionSubmitWork(t, ctx, adapter, "00000000-0000-0000-5104-000000000099")
	if err := adapter.CommitSubmitResult(ctx, work, execution.SubmitResult{Class: execution.SubmitPossiblyAccepted, ResponseFingerprint: executionDigest("d"), FinishedAt: time.Now().UTC().Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	resolution := execution.AdminResolutionCommand{
		Principal:                 execution.VerifiedPrincipal{Subject: "admin", IssuerClass: "local-test", Roles: []string{"firmware:resolve"}, Verified: true, AuthenticatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute)},
		Work:                      execution.WorkFence{CommandID: "00000000-0000-0000-5104-000000000098", ExpectedRevision: 2},
		AttemptID:                 req.FirmwareAttempt.AttemptID,
		Action:                    "RETRY_PROVEN_ABSENT",
		Reason:                    "test.proven_absent",
		EvidenceFingerprint:       executionDigest("e"),
		JournalConfirmedAbsent:    true,
		LateCallerExcluded:        true,
		LateCallerExclusionMethod: "test-fence",
		ProofReference:            "test://absence-proof",
		Now:                       time.Now().UTC().Add(2 * time.Second),
	}
	if _, err := adapter.ResolveAttention(ctx, resolution); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ResolveAttention(ctx, resolution); err != nil {
		t.Fatalf("idempotent retry of administrator resolution: %v", err)
	}
	conflict := resolution
	conflict.Reason = "test.conflicting_reason"
	if _, err := adapter.ResolveAttention(ctx, conflict); err == nil {
		t.Fatal("conflicting replay of administrator resolution unexpectedly succeeded")
	}
	conflict = resolution
	conflict.Principal.Subject = "another-admin"
	if _, err := adapter.ResolveAttention(ctx, conflict); err == nil {
		t.Fatal("replay from a different administrator unexpectedly succeeded")
	}
	journal := execution.AcceptanceJournal{RunID: "independent-run", CheckpointHash: executionDigest("f"), Entries: []execution.AcceptanceEntry{{
		AttemptID:          req.FirmwareAttempt.AttemptID,
		RequestFingerprint: req.FirmwareAttempt.RequestFingerprint,
		Received:           true,
		Sequence:           1,
	}}}
	if err := execution.VerifyPersistedJournal(context.Background(), adapter, journal); err == nil {
		t.Fatal("persisted proven-absent resolution was accepted despite independent received evidence")
	}
}

func TestAdminProvenAbsentCreatesOneFencedChildAdmission(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	initial := executionAdmissionRequest(fixture, "5105")
	if err := persistence.AdmitOperation(ctx, pool, initial); err != nil {
		t.Fatal(err)
	}

	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	submit := claimExecutionSubmitWork(t, ctx, adapter, "00000000-0000-0000-5105-000000000099")
	if err := adapter.CommitSubmitResult(ctx, submit, execution.SubmitResult{
		Class:               execution.SubmitPossiblyAccepted,
		ResponseFingerprint: executionDigest("d"),
		FinishedAt:          time.Now().UTC().Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if _, err := adapter.ResolveAttention(ctx, execution.AdminResolutionCommand{
		Principal:                 execution.VerifiedPrincipal{Subject: "admin", IssuerClass: "local-test", Roles: []string{"firmware:resolve"}, Verified: true, AuthenticatedAt: now, ExpiresAt: now.Add(time.Minute)},
		Work:                      execution.WorkFence{CommandID: "00000000-0000-0000-5105-000000000098", ExpectedRevision: 2},
		AttemptID:                 initial.FirmwareAttempt.AttemptID,
		Action:                    "RETRY_PROVEN_ABSENT",
		Reason:                    "test.proven_absent",
		EvidenceFingerprint:       executionDigest("e"),
		JournalConfirmedAbsent:    true,
		LateCallerExcluded:        true,
		LateCallerExclusionMethod: "test-fence",
		ProofReference:            "test://absence-proof",
		Now:                       now,
	}); err != nil {
		t.Fatal(err)
	}

	retry, claimed, err := adapter.ClaimPrecheckWork(ctx, "retry-test", "00000000-0000-4000-8000-000000000512")
	if err != nil || !claimed {
		t.Fatalf("claim admin retry work=%+v claimed=%t err=%v", retry, claimed, err)
	}
	child := executionAdmissionRequest(fixture, "5106")
	child.AdmissionID = initial.AdmissionID
	child.CommandID = retry.CommandID
	child.ExpectedTargetRevision = retry.ExpectedRevision
	child.CapturedCommandEpoch = 7
	child.ExpectedTargetState = "RETRY_WAIT"
	child.NextTargetState = "SUBMITTING"
	child.RetryParentAttemptID = initial.FirmwareAttempt.AttemptID
	child.ClaimedWorkID, child.ClaimedLeaseToken, child.ClaimedDeliveryGeneration = retry.DueWorkID, retry.LeaseToken, retry.DeliveryGeneration
	child.DueWorkID = "00000000-0000-0000-5106-000000000099"
	child.FirmwareAttempt.AttemptID = "00000000-0000-0000-5106-000000000010"
	child.StepAttemptID = "00000000-0000-0000-5106-000000000011"
	if err := persistence.AdmitRetryOperation(ctx, pool, child); err != nil {
		t.Fatalf("admit admin-authorized child: %v", err)
	}

	var children, retryAuthorizations, pendingSubmits, reservations int
	if err := pool.QueryRow(ctx, `SELECT retry_authorizations FROM submission_admissions WHERE id=$1::uuid`, initial.AdmissionID).Scan(&retryAuthorizations); err != nil || retryAuthorizations != 1 {
		t.Fatalf("retry authorizations=%d err=%v, want one", retryAuthorizations, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM firmware_update_attempts WHERE submission_admission_id=$1::uuid AND parent_attempt_id=$2::uuid`, initial.AdmissionID, initial.FirmwareAttempt.AttemptID).Scan(&children); err != nil || children != 1 {
		t.Fatalf("child attempts=%d err=%v, want one", children, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM operation_reservations WHERE admission_id=$1::uuid`, initial.AdmissionID).Scan(&reservations); err != nil || reservations != 3 {
		t.Fatalf("reservations=%d err=%v, want original three retained", reservations, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM due_work WHERE subject_id=$1::uuid AND work_type='SUBMIT' AND delivery_state='PENDING'`, fixture.targetUpdateID).Scan(&pendingSubmits); err != nil || pendingSubmits != 1 {
		t.Fatalf("pending child submits=%d err=%v, want one", pendingSubmits, err)
	}
	childSubmit := claimExecutionSubmitWork(t, ctx, adapter, "00000000-0000-4000-8000-000000000513")
	childAttempt, err := adapter.LoadAttempt(ctx, childSubmit)
	if err != nil {
		t.Fatalf("load claimed child submit attempt: %v", err)
	}
	if childAttempt.AttemptID != child.FirmwareAttempt.AttemptID || childAttempt.ParentAttemptID != initial.FirmwareAttempt.AttemptID || childAttempt.AdmissionID != initial.AdmissionID {
		t.Fatalf("claimed child attempt=%+v, want child=%s parent=%s admission=%s", childAttempt, child.FirmwareAttempt.AttemptID, initial.FirmwareAttempt.AttemptID, initial.AdmissionID)
	}
	if err := persistence.AdmitRetryOperation(ctx, pool, child); err == nil {
		t.Fatal("duplicate admin-authorized child admission unexpectedly succeeded")
	}
}
