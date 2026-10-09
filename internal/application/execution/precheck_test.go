package execution

import (
	"context"
	"testing"
	"time"
)

type precheckRepositoryStub struct {
	snapshot   PrecheckSnapshot
	admissions int
}

func (s *precheckRepositoryStub) LoadPrecheck(context.Context, WorkFence) (PrecheckSnapshot, error) {
	return s.snapshot, nil
}
func (s *precheckRepositoryStub) AdmitSubmission(context.Context, AdmissionCommand) (AdmissionResult, error) {
	s.admissions++
	return AdmissionResult{AttemptID: "attempt"}, nil
}

type precheckTargetStub struct{ evidence PrecheckEvidence }

func (s precheckTargetStub) Precheck(context.Context, PrecheckRequest) (PrecheckEvidence, error) {
	return s.evidence, nil
}

func precheckWork() WorkFence {
	return WorkFence{DueWorkID: "work", CommandID: "command", LeaseToken: "lease", ExpectedRevision: 4}
}

func precheckSnapshot() PrecheckSnapshot {
	return PrecheckSnapshot{TargetID: "target", CampaignID: "campaign", TargetUpdateID: "update", ArtifactID: "artifact", ExpectedVersion: "2.0", CommandEpoch: 3, Revision: 4}
}

func precheckCommand() AdmissionCommand {
	return AdmissionCommand{AttemptID: "attempt", StepAttemptID: "step", AdmissionID: "admission"}
}

func TestPrecheckAndAdmitRequiresEligibleFreshEvidence(t *testing.T) {
	repository := &precheckRepositoryStub{snapshot: precheckSnapshot()}
	evidence := PrecheckEvidence{CurrentVersion: "1.0", Capable: true, Healthy: true, Compatible: true, EvidenceFingerprint: "sha256:evidence", ObservedAt: time.Now().UTC()}
	result, _, err := PrecheckAndAdmit(context.Background(), repository, precheckTargetStub{evidence: evidence}, precheckWork(), precheckCommand(), "credential")
	if err != nil || result.AttemptID != "attempt" || repository.admissions != 1 {
		t.Fatalf("result=%+v err=%v admissions=%d", result, err, repository.admissions)
	}
}

func TestPrecheckAndAdmitNeverAdmitsAlreadyCurrentTarget(t *testing.T) {
	repository := &precheckRepositoryStub{snapshot: precheckSnapshot()}
	evidence := PrecheckEvidence{CurrentVersion: "2.0", Capable: true, Healthy: true, Compatible: true, EvidenceFingerprint: "sha256:evidence", ObservedAt: time.Now().UTC()}
	if _, _, err := PrecheckAndAdmit(context.Background(), repository, precheckTargetStub{evidence: evidence}, precheckWork(), precheckCommand(), "credential"); err == nil || repository.admissions != 0 {
		t.Fatalf("err=%v admissions=%d", err, repository.admissions)
	}
}

func TestPrecheckAndAdmitNeverAdmitsIneligibleTarget(t *testing.T) {
	repository := &precheckRepositoryStub{snapshot: precheckSnapshot()}
	evidence := PrecheckEvidence{CurrentVersion: "1.0", Capable: true, Healthy: false, Compatible: true, EvidenceFingerprint: "sha256:evidence", ObservedAt: time.Now().UTC()}
	if _, _, err := PrecheckAndAdmit(context.Background(), repository, precheckTargetStub{evidence: evidence}, precheckWork(), precheckCommand(), "credential"); err == nil || repository.admissions != 0 {
		t.Fatalf("err=%v admissions=%d", err, repository.admissions)
	}
}
