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

func (writer *declarationWriter) InitiateUpload(context.Context, UploadDeclaration) (string, error) {
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
