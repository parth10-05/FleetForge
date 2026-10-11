package execution

import (
	"context"
	"testing"
	"time"
)

type precheckClaimer struct{ work WorkFence }

func (c precheckClaimer) ClaimPrecheckWork(context.Context, string, string) (WorkFence, bool, error) {
	return c.work, true, nil
}

type precheckCommandSource struct {
	command    AdmissionCommand
	credential string
}

func (s precheckCommandSource) LoadPrecheckCommand(context.Context, WorkFence) (AdmissionCommand, string, error) {
	return s.command, s.credential, nil
}

func TestPrecheckWorkerClaimsOnlyOneFencedPrecheckAndCompletesAlreadyCurrent(t *testing.T) {
	work := precheckWork()
	repository := &precheckRepositoryStub{snapshot: precheckSnapshot()}
	command := precheckCommand()
	command.AlreadyCurrentEnvelope = alreadyCurrentEnvelope()
	worker := PrecheckWorker{
		Claimer: precheckClaimer{work: work}, Repository: repository,
		Target:   precheckTargetStub{evidence: PrecheckEvidence{CurrentVersion: "2.0", Healthy: true, EvidenceFingerprint: "sha256:evidence", ObservedAt: time.Now().UTC()}},
		Commands: precheckCommandSource{command: command, credential: "credential"},
	}
	claimed, err := worker.RunOnce(context.Background(), "precheck-worker", "lease")
	if err != nil || !claimed || repository.completions != 1 || repository.admissions != 0 {
		t.Fatalf("claimed=%t err=%v completions=%d admissions=%d", claimed, err, repository.completions, repository.admissions)
	}
}
