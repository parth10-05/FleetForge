package artifacts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

func TestVerificationWorkerCompletesOnlyVerifiedLease(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	service := verificationService(t, nil)
	work := VerificationWork{ID: "work-1", ArtifactID: "artifact-1", LeaseToken: "lease-1", WorkType: VerificationWorkType, ExpectedRevision: 2}
	finisher := &workFinisher{}
	worker := NewWorker(service, workClaimer{work: []VerificationWork{work}}, finisher, func() time.Time { return now })
	if err := worker.RunOnce(context.Background(), VerificationWorkClaimRequest{Owner: "worker-a", LeaseToken: "lease-1", Now: now, LeaseExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if finisher.completed != 1 || finisher.retried != 0 || finisher.work.LeaseToken != work.LeaseToken {
		t.Fatalf("finisher=%+v", finisher)
	}
}

func TestVerificationWorkerRetriesLeaseWhenVerificationCannotRun(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	service := New(nil, nil, nil).WithVerification(verificationReaderFunc(func(context.Context, string) (artifact.ObjectIdentity, error) {
		return artifact.ObjectIdentity{}, errors.New("object unavailable")
	}), verificationWriterFunc(func(context.Context, string, artifact.ObjectIdentity, bool, string) error { return nil }))
	service.verifier = verifierFunc(func(context.Context, artifact.ObjectIdentity) error { return nil })
	work := VerificationWork{ID: "work-1", ArtifactID: "artifact-1", LeaseToken: "lease-1", WorkType: VerificationWorkType, ExpectedRevision: 2}
	finisher := &workFinisher{}
	worker := NewWorker(service, workClaimer{work: []VerificationWork{work}}, finisher, func() time.Time { return now })
	if err := worker.RunOnce(context.Background(), VerificationWorkClaimRequest{Owner: "worker-a", LeaseToken: "lease-1", Now: now, LeaseExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if finisher.completed != 0 || finisher.retried != 1 || !finisher.retryAt.Equal(now.Add(time.Second)) {
		t.Fatalf("finisher=%+v", finisher)
	}
}

func verificationService(t *testing.T, verifyErr error) *Service {
	t.Helper()
	metadata := testMetadata(t)
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "objects/a.bin", "version-1", metadata.Size(), metadata.Digest())
	if err != nil {
		t.Fatal(err)
	}
	service := New(nil, nil, nil).WithVerification(verificationReaderFunc(func(context.Context, string) (artifact.ObjectIdentity, error) { return object, nil }), verificationWriterFunc(func(context.Context, string, artifact.ObjectIdentity, bool, string) error { return nil }))
	service.verifier = verifierFunc(func(context.Context, artifact.ObjectIdentity) error { return verifyErr })
	return service
}

type workClaimer struct{ work []VerificationWork }

func (claimer workClaimer) ClaimVerificationWork(context.Context, VerificationWorkClaimRequest) ([]VerificationWork, error) {
	return claimer.work, nil
}

type workFinisher struct {
	completed, retried int
	work               VerificationWork
	retryAt            time.Time
}

func (finisher *workFinisher) CompleteVerificationWork(_ context.Context, work VerificationWork, _ time.Time) error {
	finisher.completed++
	finisher.work = work
	return nil
}
func (finisher *workFinisher) RetryVerificationWork(_ context.Context, work VerificationWork, _ time.Time, retryAt time.Time) error {
	finisher.retried++
	finisher.work, finisher.retryAt = work, retryAt
	return nil
}
