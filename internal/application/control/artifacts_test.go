package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	artifactapp "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestArtifactServiceUsesStableVersionOnlyAfterAuthorizedPersistence(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	digest, _ := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	metadata, _ := artifact.NewUploadMetadata("firmware.bin", artifact.SyntheticFirmwareMediaType, 3, digest)
	store := &recordingArtifactInitiator{result: ArtifactInitiationResult{ArtifactID: "10000000-0000-0000-0000-000000000001", CommandID: "10000000-0000-0000-0000-000000000002", GrantID: "grant-1", Key: "artifacts/v1/a.bin", AcceptedAt: now, ExpiresAt: now.Add(time.Minute), Metadata: metadata}}
	issuer := &recordingArtifactIssuer{}
	service := NewArtifactService(nil, func() time.Time { return now }).WithInitiation(store, issuer, "artifacts")
	admin := access.Principal{Subject: "admin", Roles: []access.Role{access.RoleAdmin}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}
	meta := CommandMeta{ClaimID: "10000000-0000-0000-0000-000000000003", CommandID: "10000000-0000-0000-0000-000000000002", CorrelationID: "10000000-0000-0000-0000-000000000004", OwnerToken: "10000000-0000-0000-0000-000000000005", OutboxEventID: "10000000-0000-0000-0000-000000000006", DecisionInputID: "10000000-0000-0000-0000-000000000007", Route: "/v1/artifacts:initiate", IdempotencyKey: "artifact-initiation-request", CanonicalRequest: []byte(`{}`)}
	result, receipt, err := service.InitiateArtifact(context.Background(), admin, meta, InitiateArtifactCommand{ArtifactID: store.result.ArtifactID, GrantID: store.result.GrantID, FirmwareVersion: "v1", Metadata: metadata, ExpiresAt: now.Add(time.Minute)})
	if err != nil || result.ArtifactID != store.result.ArtifactID || receipt.Authorization != "signed-grant" || store.calls != 1 || issuer.calls != 1 || !strings.HasPrefix(issuer.request.VersionID, "api-") {
		t.Fatalf("result=%+v receipt=%+v calls=%d/%d version=%q err=%v", result, receipt, store.calls, issuer.calls, issuer.request.VersionID, err)
	}
	viewer := admin
	viewer.Roles = []access.Role{access.RoleViewer}
	if _, _, err := service.InitiateArtifact(context.Background(), viewer, meta, InitiateArtifactCommand{ArtifactID: store.result.ArtifactID, GrantID: store.result.GrantID, FirmwareVersion: "v1", Metadata: metadata, ExpiresAt: now.Add(time.Minute)}); err == nil || store.calls != 1 {
		t.Fatalf("viewer initiation err=%v calls=%d", err, store.calls)
	}
}

type recordingArtifactInitiator struct {
	result ArtifactInitiationResult
	calls  int
}

func (store *recordingArtifactInitiator) InitiateArtifact(_ context.Context, _ access.Principal, _ CommandMeta, _ InitiateArtifactCommand) (ArtifactInitiationResult, error) {
	store.calls++
	return store.result, nil
}

type recordingArtifactIssuer struct {
	request artifactapp.UploadCapabilityRequest
	calls   int
}

func (issuer *recordingArtifactIssuer) IssueUploadCapability(_ context.Context, request artifactapp.UploadCapabilityRequest) (artifactapp.UploadCapability, error) {
	issuer.calls++
	issuer.request = request
	object, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, request.Container, request.Key, request.VersionID, request.Metadata.Size(), request.Metadata.Digest())
	return artifactapp.UploadCapability{Authorization: "signed-grant", Object: object, ExpiresAt: request.ExpiresAt}, nil
}
