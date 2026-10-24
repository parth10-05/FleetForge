package execution

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

type submitRepo struct {
	attempt   AttemptSnapshot
	committed int
}

func (r *submitRepo) LoadPrecheck(context.Context, WorkFence) (PrecheckSnapshot, error) {
	panic("unused")
}
func (r *submitRepo) AdmitSubmission(context.Context, AdmissionCommand) (AdmissionResult, error) {
	panic("unused")
}
func (r *submitRepo) AdmitRetrySubmission(context.Context, AdmissionCommand) (AdmissionResult, error) {
	panic("unused")
}
func (r *submitRepo) LoadAttempt(context.Context, WorkFence) (AttemptSnapshot, error) {
	return r.attempt, nil
}
func (r *submitRepo) CommitSubmitResult(context.Context, WorkFence, SubmitResult) error {
	r.committed++
	return nil
}
func (r *submitRepo) CommitObservation(context.Context, WorkFence, ObservationDecision) error {
	panic("unused")
}
func (r *submitRepo) LoadReconciliation(context.Context, WorkFence) (ReconciliationSnapshot, error) {
	panic("unused")
}
func (r *submitRepo) CommitReconciliation(context.Context, WorkFence, ReconciliationDecision) error {
	panic("unused")
}
func (r *submitRepo) ResolveAttention(context.Context, AdminResolutionCommand) (ResolutionResult, error) {
	panic("unused")
}

type submitTarget struct{ calls int }

func (t *submitTarget) Precheck(context.Context, PrecheckRequest) (PrecheckEvidence, error) {
	panic("unused")
}
func (t *submitTarget) Submit(_ context.Context, _ SubmitRequest, body io.Reader) SubmitResult {
	t.calls++
	_, _ = io.ReadAll(body)
	return SubmitResult{Class: SubmitPossiblyAccepted, FinishedAt: time.Now()}
}
func (t *submitTarget) ObserveTask(context.Context, TaskObservationRequest) (Observation, error) {
	panic("unused")
}
func (t *submitTarget) ObserveInventory(context.Context, InventoryObservationRequest) (Observation, error) {
	panic("unused")
}
func (t *submitTarget) ObserveHealth(context.Context, HealthObservationRequest) (Observation, error) {
	panic("unused")
}
func TestSubmitOnceCallsTargetAndCommitsOneClosedResult(t *testing.T) {
	object := ObjectIdentity{Provider: "local", Container: "c", ObjectKey: "k", ObjectVersion: "v", Size: 3, SHA256: "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}
	repo := &submitRepo{attempt: AttemptSnapshot{AttemptID: "attempt", Artifact: object}}
	target := &submitTarget{}
	reader := exactReaderFunc(func(context.Context, ObjectIdentity) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("abc"))), nil
	})
	err := SubmitOnce(context.Background(), repo, reader, target, WorkFence{DueWorkID: "work", CommandID: "command", LeaseToken: "lease"}, "/redfish/v1/UpdateService/Actions/UpdateService.SimpleUpdate")
	if err != nil || target.calls != 1 || repo.committed != 1 {
		t.Fatalf("err=%v calls=%d commits=%d", err, target.calls, repo.committed)
	}
}
