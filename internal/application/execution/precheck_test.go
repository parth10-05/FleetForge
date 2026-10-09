package execution

import (
	"context"
	"testing"
	"time"
)

type precheckRepositoryStub struct {
	snapshot   PrecheckSnapshot
	admissions int
	command    AdmissionCommand
}

func (s *precheckRepositoryStub) LoadPrecheck(context.Context, WorkFence) (PrecheckSnapshot, error) {
	return s.snapshot, nil
}
func (s *precheckRepositoryStub) AdmitSubmission(_ context.Context, command AdmissionCommand) (AdmissionResult, error) {
	s.admissions++
	s.command = command
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
	now := time.Now().UTC()
	return AdmissionCommand{AttemptID: "attempt", StepAttemptID: "step", AdmissionID: "admission", RequestFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Artifact: ObjectIdentity{Provider: "local", Container: "firmware", ObjectKey: "verified.bin", ObjectVersion: "v1", SHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 3}, Envelope: AdmissionEnvelope{CampaignTargetID: "campaign-target", ExpectedTargetState: "PRECHECK", NextTargetState: "SUBMITTING", Step: "SUBMISSION", Global: CapacityLimit{Level: "GLOBAL", Key: "GLOBAL", Limit: 1}, Site: CapacityLimit{Level: "SITE", Key: "site-a", Limit: 1}, Rack: CapacityLimit{Level: "RACK", Key: "rack-a", Limit: 1}, ReservationIDs: [3]string{"global", "site", "rack"}, ActorID: "system", CorrelationID: "correlation", CausationID: "causation", Action: "SUBMISSION_ADMITTED", ReasonCode: "target.submission.authorized", OutboxEventID: "outbox", OutboxEventType: "target.submission_admitted", OutboxSchemaVersion: 1, OutboxPayload: []byte(`{"admitted":true}`), DueWorkID: "submit-work", DueWorkWorkType: "SUBMIT", DueWorkSchemaVersion: 1, DueWorkNotBefore: now, DecisionInputID: "decision", DecisionLogicalTime: now, ReducerVersion: "domain-v1", DecisionPolicyVersion: "policy-v1", NormalizedFacts: []byte(`{"eligible":true}`), DecisionResult: "SUBMITTING"}}
}

func TestPrecheckAndAdmitRequiresExactImmutableArtifact(t *testing.T) {
	repository := &precheckRepositoryStub{snapshot: precheckSnapshot()}
	evidence := PrecheckEvidence{CurrentVersion: "1.0", Capable: true, Healthy: true, Compatible: true, EvidenceFingerprint: "sha256:evidence", ObservedAt: time.Now().UTC()}
	command := precheckCommand()
	command.Artifact.ObjectVersion = ""
	if _, _, err := PrecheckAndAdmit(context.Background(), repository, precheckTargetStub{evidence: evidence}, precheckWork(), command, "credential"); err == nil || repository.admissions != 0 {
		t.Fatalf("err=%v admissions=%d", err, repository.admissions)
	}
}

func TestPrecheckAndAdmitRequiresJournalMatchableRequestFingerprint(t *testing.T) {
	repository := &precheckRepositoryStub{snapshot: precheckSnapshot()}
	evidence := PrecheckEvidence{CurrentVersion: "1.0", Capable: true, Healthy: true, Compatible: true, EvidenceFingerprint: "sha256:evidence", ObservedAt: time.Now().UTC()}
	command := precheckCommand()
	command.RequestFingerprint = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, _, err := PrecheckAndAdmit(context.Background(), repository, precheckTargetStub{evidence: evidence}, precheckWork(), command, "credential"); err == nil || repository.admissions != 0 {
		t.Fatalf("err=%v admissions=%d", err, repository.admissions)
	}
}

func TestPrecheckAndAdmitRequiresEligibleFreshEvidence(t *testing.T) {
	repository := &precheckRepositoryStub{snapshot: precheckSnapshot()}
	evidence := PrecheckEvidence{CurrentVersion: "1.0", Capable: true, Healthy: true, Compatible: true, EvidenceFingerprint: "sha256:evidence", ObservedAt: time.Now().UTC()}
	result, _, err := PrecheckAndAdmit(context.Background(), repository, precheckTargetStub{evidence: evidence}, precheckWork(), precheckCommand(), "credential")
	if err != nil || result.AttemptID != "attempt" || repository.admissions != 1 {
		t.Fatalf("result=%+v err=%v admissions=%d", result, err, repository.admissions)
	}
	if repository.command.PrecheckEvidence.EvidenceFingerprint != evidence.EvidenceFingerprint || repository.command.PrecheckEvidence.ObservedAt != evidence.ObservedAt {
		t.Fatalf("admission command did not retain precheck evidence: %#v", repository.command.PrecheckEvidence)
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
