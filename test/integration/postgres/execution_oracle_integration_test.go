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
	if _, err := adapter.ResolveAttention(ctx, execution.AdminResolutionCommand{
		Principal:                 execution.VerifiedPrincipal{Subject: "admin", IssuerClass: "local-test", Verified: true},
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
	}); err != nil {
		t.Fatal(err)
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
