package execution

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/parth10-05/fleetforge/internal/application/orchestration"
)

func TestWakeupSubmitHandlerExecutesOnlyClaimedSubmitWork(t *testing.T) {
	object := ObjectIdentity{Provider: "local", Container: "c", ObjectKey: "k", ObjectVersion: "v", Size: 3, SHA256: "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}
	repository := &submitRepo{attempt: AttemptSnapshot{AttemptID: "attempt", Artifact: object}}
	target := &submitTarget{}
	handler := WakeupSubmitHandler{
		Repository: repository,
		Artifacts: exactReaderFunc(func(context.Context, ObjectIdentity) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("abc"))), nil
		}),
		Target:     target,
		ActionPath: "UpdateService/update",
	}
	envelope := orchestration.Envelope{QueueClass: orchestration.QueueUpdateStep, WorkType: "SUBMIT"}
	if !handler.Supports(envelope) {
		t.Fatal("submit wakeup was not supported")
	}
	if handler.Supports(orchestration.Envelope{QueueClass: orchestration.QueueUpdateStep, WorkType: "TASK_OBSERVE"}) {
		t.Fatal("observation wakeup must not have a submission route")
	}
	if err := handler.HandleFinalized(context.Background(), orchestration.ClaimedWork{DueWorkID: "work", CommandID: "command", LeaseToken: "lease", WorkType: "SUBMIT"}); err != nil {
		t.Fatal(err)
	}
	if target.calls != 1 || repository.committed != 1 {
		t.Fatalf("target=%d commits=%d", target.calls, repository.committed)
	}
}

func TestWakeupSubmitHandlerRejectsMismatchedClaim(t *testing.T) {
	handler := WakeupSubmitHandler{Repository: &submitRepo{}, Artifacts: exactReaderFunc(func(context.Context, ObjectIdentity) (io.ReadCloser, error) { return nil, nil }), Target: &submitTarget{}, ActionPath: "UpdateService/update"}
	if err := handler.HandleFinalized(context.Background(), orchestration.ClaimedWork{WorkType: "TASK_OBSERVE"}); err == nil {
		t.Fatal("non-submit work was accepted")
	}
}
