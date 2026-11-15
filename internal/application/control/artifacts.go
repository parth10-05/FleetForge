package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	artifactapp "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

var ErrArtifactNotFound = errors.New("artifact projection not found")

type ArtifactProjection struct {
	ArtifactID string
	State      string
	AsOf       time.Time
}

type ArtifactProjectionReader interface {
	GetArtifact(context.Context, string) (ArtifactProjection, error)
}

type InitiateArtifactCommand struct {
	ArtifactID, GrantID, FirmwareVersion string
	Metadata                             artifact.UploadMetadata
	ExpiresAt                            time.Time
}

type ArtifactInitiationResult struct {
	ArtifactID, CommandID, GrantID, Key string
	AcceptedAt, ExpiresAt               time.Time
	Metadata                            artifact.UploadMetadata
}

type ArtifactInitiationStore interface {
	InitiateArtifact(context.Context, access.Principal, CommandMeta, InitiateArtifactCommand) (ArtifactInitiationResult, error)
}

type ArtifactService struct {
	store     ArtifactProjectionReader
	initiator ArtifactInitiationStore
	issuer    artifactapp.UploadCapabilityIssuer
	container string
	now       func() time.Time
}

func NewArtifactService(store ArtifactProjectionReader, now func() time.Time) *ArtifactService {
	if now == nil {
		now = time.Now
	}
	return &ArtifactService{store: store, now: now}
}

func (service *ArtifactService) WithInitiation(initiator ArtifactInitiationStore, issuer artifactapp.UploadCapabilityIssuer, container string) *ArtifactService {
	service.initiator, service.issuer, service.container = initiator, issuer, container
	return service
}

func (service *ArtifactService) GetArtifact(ctx context.Context, principal access.Principal, artifactID string) (ArtifactProjection, error) {
	if service == nil || service.store == nil || artifactID == "" {
		return ArtifactProjection{}, fmt.Errorf("artifact projection persistence is unavailable")
	}
	if err := access.Authorize(principal, access.PermissionReadArtifacts, service.now()); err != nil {
		return ArtifactProjection{}, err
	}
	return service.store.GetArtifact(ctx, artifactID)
}

func (service *ArtifactService) InitiateArtifact(ctx context.Context, principal access.Principal, meta CommandMeta, command InitiateArtifactCommand) (ArtifactInitiationResult, artifactapp.UploadReceipt, error) {
	if service == nil || service.initiator == nil || service.issuer == nil || service.container == "" || meta.ClaimID == "" || meta.CommandID == "" || meta.CorrelationID == "" || meta.OwnerToken == "" || meta.OutboxEventID == "" || meta.DecisionInputID == "" || meta.Route != "/v1/artifacts:initiate" || meta.IdempotencyKey == "" || len(meta.CanonicalRequest) == 0 || command.ArtifactID == "" || command.GrantID == "" || command.FirmwareVersion == "" || command.ExpiresAt.IsZero() {
		return ArtifactInitiationResult{}, artifactapp.UploadReceipt{}, fmt.Errorf("invalid artifact initiation command")
	}
	now := service.now().UTC()
	if !command.ExpiresAt.After(now) || command.ExpiresAt.After(now.Add(5*time.Minute)) {
		return ArtifactInitiationResult{}, artifactapp.UploadReceipt{}, fmt.Errorf("invalid artifact initiation command")
	}
	if err := access.Authorize(principal, access.PermissionInitiateArtifacts, service.now()); err != nil {
		return ArtifactInitiationResult{}, artifactapp.UploadReceipt{}, err
	}
	result, err := service.initiator.InitiateArtifact(ctx, principal, meta, command)
	if err != nil {
		return ArtifactInitiationResult{}, artifactapp.UploadReceipt{}, err
	}
	versionID := stableUploadVersionID(result.GrantID)
	capability, err := service.issuer.IssueUploadCapability(ctx, artifactapp.UploadCapabilityRequest{ArtifactID: result.ArtifactID, GrantID: result.GrantID, Container: service.container, Key: result.Key, VersionID: versionID, Metadata: result.Metadata, ExpiresAt: result.ExpiresAt})
	if err != nil {
		return ArtifactInitiationResult{}, artifactapp.UploadReceipt{}, fmt.Errorf("issue upload capability: %w", err)
	}
	if capability.Authorization == "" || !capability.ExpiresAt.UTC().Equal(result.ExpiresAt.UTC()) || capability.Object.Key() != result.Key || capability.Object.Size() != result.Metadata.Size() || capability.Object.Digest() != result.Metadata.Digest() {
		return ArtifactInitiationResult{}, artifactapp.UploadReceipt{}, fmt.Errorf("issue upload capability: invalid response")
	}
	return result, artifactapp.UploadReceipt{Authorization: capability.Authorization, Object: capability.Object, ExpiresAt: capability.ExpiresAt}, nil
}

func stableUploadVersionID(grantID string) string {
	digest := sha256.Sum256([]byte(grantID))
	return "api-" + hex.EncodeToString(digest[:16])
}
