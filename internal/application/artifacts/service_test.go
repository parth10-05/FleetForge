package artifacts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestInitiateUploadReservesBeforeIssuingExactCapability(t *testing.T) {
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	metadata := testMetadata(t)
	declarations := &declarationWriter{key: "artifacts/v1/00000000-0000-0000-2401-000000000001.bin"}
	issuer := &capabilityIssuer{}
	service := New(declarations, issuer, func() time.Time { return now })
	result, err := service.InitiateUpload(context.Background(), InitiateUploadRequest{Declaration: UploadDeclaration{ArtifactID: "00000000-0000-0000-2401-000000000001", FirmwareVersion: "v1", Metadata: metadata}, GrantID: "grant-1", Container: "artifacts", ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if declarations.calls != 1 || issuer.calls != 1 || result.Object.Key() != declarations.key || result.Authorization == "" {
		t.Fatalf("declarations=%d issuer=%d result=%+v", declarations.calls, issuer.calls, result)
	}
}

func TestInitiateUploadRejectsIssuerSubstitution(t *testing.T) {
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	service := New(&declarationWriter{key: "artifacts/v1/00000000-0000-0000-2401-000000000001.bin"}, &capabilityIssuer{substitute: true}, func() time.Time { return now })
	_, err := service.InitiateUpload(context.Background(), InitiateUploadRequest{Declaration: UploadDeclaration{ArtifactID: "00000000-0000-0000-2401-000000000001", FirmwareVersion: "v1", Metadata: testMetadata(t)}, GrantID: "grant-1", Container: "artifacts", ExpiresAt: now.Add(time.Minute)})
	if !errors.Is(err, ErrInvalidUploadRequest) {
		t.Fatalf("substitution error=%v", err)
	}
}

func TestInitiateUploadDoesNotIssueBeforeReservationSucceeds(t *testing.T) {
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	issuer := &capabilityIssuer{}
	service := New(&declarationWriter{err: errors.New("database unavailable")}, issuer, func() time.Time { return now })
	_, err := service.InitiateUpload(context.Background(), InitiateUploadRequest{Declaration: UploadDeclaration{ArtifactID: "00000000-0000-0000-2401-000000000001", FirmwareVersion: "v1", Metadata: testMetadata(t)}, GrantID: "grant-1", Container: "artifacts", ExpiresAt: now.Add(time.Minute)})
	if err == nil || issuer.calls != 0 {
		t.Fatalf("err=%v issuer=%d", err, issuer.calls)
	}
}

func testMetadata(t *testing.T) artifact.UploadMetadata {
	t.Helper()
	digest, err := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := artifact.NewUploadMetadata("firmware.bin", artifact.SyntheticFirmwareMediaType, 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

type declarationWriter struct {
	key   string
	err   error
	calls int
}

func (writer *declarationWriter) InitiateUpload(context.Context, UploadDeclaration, string) (string, error) {
	writer.calls++
	return writer.key, writer.err
}

type capabilityIssuer struct {
	calls      int
	substitute bool
}

func (issuer *capabilityIssuer) IssueUploadCapability(_ context.Context, request UploadCapabilityRequest) (UploadCapability, error) {
	issuer.calls++
	key := request.Key
	if issuer.substitute {
		key = "artifacts/v1/substituted.bin"
	}
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, request.Container, key, "local-version-1", request.Metadata.Size(), request.Metadata.Digest())
	if err != nil {
		return UploadCapability{}, err
	}
	return UploadCapability{Authorization: "opaque-grant", Object: object, ExpiresAt: request.ExpiresAt}, nil
}

func TestCompleteUploadVerifiesBeforeBinding(t *testing.T) {
	metadata := testMetadata(t)
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/a.bin", "local-1", metadata.Size(), metadata.Digest())
	if err != nil {
		t.Fatal(err)
	}
	sequence := []string{}
	service := NewWithCompletion(nil, nil, verifierFunc(func(context.Context, artifact.ObjectIdentity) error {
		sequence = append(sequence, "verify")
		return nil
	}), binderFunc(func(context.Context, string, artifact.ObjectIdentity) error {
		sequence = append(sequence, "bind")
		return nil
	}), nil)
	if err := service.CompleteUpload(context.Background(), "00000000-0000-0000-2401-000000000001", object); err != nil {
		t.Fatal(err)
	}
	if strings.Join(sequence, ",") != "verify,bind" {
		t.Fatalf("sequence=%v", sequence)
	}
}

func TestCompleteUploadDoesNotBindUnverifiedObject(t *testing.T) {
	metadata := testMetadata(t)
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/a.bin", "local-1", metadata.Size(), metadata.Digest())
	if err != nil {
		t.Fatal(err)
	}
	binderCalls := 0
	service := NewWithCompletion(nil, nil, verifierFunc(func(context.Context, artifact.ObjectIdentity) error { return errors.New("not present") }), binderFunc(func(context.Context, string, artifact.ObjectIdentity) error { binderCalls++; return nil }), nil)
	if err := service.CompleteUpload(context.Background(), "00000000-0000-0000-2401-000000000001", object); err == nil || binderCalls != 0 {
		t.Fatalf("err=%v binder=%d", err, binderCalls)
	}
}

type verifierFunc func(context.Context, artifact.ObjectIdentity) error

func (fn verifierFunc) VerifyObject(ctx context.Context, object artifact.ObjectIdentity) error {
	return fn(ctx, object)
}

type binderFunc func(context.Context, string, artifact.ObjectIdentity) error

func (fn binderFunc) BindUploadedObject(ctx context.Context, id string, object artifact.ObjectIdentity) error {
	return fn(ctx, id, object)
}

func TestVerifyBoundArtifactRecordsExactSuccess(t *testing.T) {
	metadata := testMetadata(t)
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/a.bin", "local-1", metadata.Size(), metadata.Digest())
	if err != nil {
		t.Fatal(err)
	}
	recorded := verificationRecord{}
	service := New(nil, nil, nil).WithVerification(verificationReaderFunc(func(context.Context, string) (artifact.ObjectIdentity, error) { return object, nil }), verificationWriterFunc(func(_ context.Context, id string, got artifact.ObjectIdentity, verified bool, reason string) error {
		recorded = verificationRecord{id, got, verified, reason}
		return nil
	}))
	service.verifier = verifierFunc(func(context.Context, artifact.ObjectIdentity) error { return nil })
	if err := service.VerifyBoundArtifact(context.Background(), "artifact-1"); err != nil {
		t.Fatal(err)
	}
	if !recorded.verified || recorded.reason != "digest_exact" || recorded.object != object {
		t.Fatalf("record=%+v", recorded)
	}
}

func TestVerifyBoundArtifactRecordsRejectedExactVersion(t *testing.T) {
	metadata := testMetadata(t)
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/a.bin", "local-1", metadata.Size(), metadata.Digest())
	if err != nil {
		t.Fatal(err)
	}
	recorded := verificationRecord{}
	service := New(nil, nil, nil).WithVerification(verificationReaderFunc(func(context.Context, string) (artifact.ObjectIdentity, error) { return object, nil }), verificationWriterFunc(func(_ context.Context, id string, got artifact.ObjectIdentity, verified bool, reason string) error {
		recorded = verificationRecord{id, got, verified, reason}
		return nil
	}))
	service.verifier = verifierFunc(func(context.Context, artifact.ObjectIdentity) error { return errors.New("digest mismatch") })
	if err := service.VerifyBoundArtifact(context.Background(), "artifact-1"); err != nil {
		t.Fatal(err)
	}
	if recorded.verified || recorded.reason != "artifact.verification.failed" || recorded.object != object {
		t.Fatalf("record=%+v", recorded)
	}
}

type verificationReaderFunc func(context.Context, string) (artifact.ObjectIdentity, error)

func (fn verificationReaderFunc) LoadVerificationArtifact(ctx context.Context, id string) (artifact.ObjectIdentity, error) {
	return fn(ctx, id)
}

type verificationWriterFunc func(context.Context, string, artifact.ObjectIdentity, bool, string) error

func (fn verificationWriterFunc) RecordVerification(ctx context.Context, id string, object artifact.ObjectIdentity, verified bool, reason string) error {
	return fn(ctx, id, object, verified, reason)
}

type verificationRecord struct {
	id       string
	object   artifact.ObjectIdentity
	verified bool
	reason   string
}
