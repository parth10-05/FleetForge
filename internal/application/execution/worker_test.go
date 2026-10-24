package execution

import (
	"bytes"
	"context"
	"io"
	"testing"
)

type submitClaimer struct {
	work    WorkFence
	claimed bool
	calls   int
}

func (c *submitClaimer) ClaimSubmitWork(context.Context, string, string) (WorkFence, bool, error) {
	c.calls++
	return c.work, c.claimed, nil
}

func TestSubmitWorkerClaimsAndExecutesAtMostOneFencedWork(t *testing.T) {
	object := ObjectIdentity{Provider: "local", Container: "c", ObjectKey: "k", ObjectVersion: "v", Size: 3, SHA256: "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}
	repository := &submitRepo{attempt: AttemptSnapshot{AttemptID: "attempt", Artifact: object}}
	target := &submitTarget{}
	claimer := &submitClaimer{work: WorkFence{DueWorkID: "work", CommandID: "command", LeaseToken: "lease"}, claimed: true}
	worker := SubmitWorker{Claimer: claimer, Repository: repository, Artifacts: exactReaderFunc(func(context.Context, ObjectIdentity) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("abc"))), nil
	}), Target: target, ActionPath: "UpdateService/update"}
	claimed, err := worker.RunOnce(context.Background(), "worker", "lease")
	if err != nil || !claimed || claimer.calls != 1 || target.calls != 1 || repository.committed != 1 {
		t.Fatalf("claimed=%t err=%v calls=%d target=%d commits=%d", claimed, err, claimer.calls, target.calls, repository.committed)
	}
}
