// Package artifacts coordinates P02 artifact declaration and direct-upload
// capability issuance. It never accepts firmware bytes or storage credentials.
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

var ErrInvalidUploadRequest = errors.New("invalid artifact upload request")

type UploadDeclaration struct {
	ArtifactID      string
	FirmwareVersion string
	Metadata        artifact.UploadMetadata
}

type UploadDeclarationWriter interface {
	InitiateUpload(context.Context, UploadDeclaration) (string, error)
}

// UploadCapabilityRequest contains only the exact facts a direct capability
// may authorize. The issuer owns signing and provider version assignment.
type UploadCapabilityRequest struct {
	GrantID   string
	Container string
	Key       string
	Metadata  artifact.UploadMetadata
	ExpiresAt time.Time
}

type UploadCapability struct {
	Authorization string
	Object        artifact.ObjectIdentity
	ExpiresAt     time.Time
}

type UploadCapabilityIssuer interface {
	IssueUploadCapability(context.Context, UploadCapabilityRequest) (UploadCapability, error)
}

type InitiateUploadRequest struct {
	Declaration UploadDeclaration
	GrantID     string
	Container   string
	ExpiresAt   time.Time
}

type UploadReceipt struct {
	Authorization string
	Object        artifact.ObjectIdentity
	ExpiresAt     time.Time
}

type Service struct {
	declarations UploadDeclarationWriter
	issuer       UploadCapabilityIssuer
	now          func() time.Time
}

func New(declarations UploadDeclarationWriter, issuer UploadCapabilityIssuer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{declarations: declarations, issuer: issuer, now: now}
}

// InitiateUpload reserves the generated artifact key before issuing one
// direct-upload capability. It validates the returned immutable tuple to stop
// a miswired issuer from authorizing a sibling key or altered declaration.
func (service *Service) InitiateUpload(ctx context.Context, request InitiateUploadRequest) (UploadReceipt, error) {
	if service == nil || service.declarations == nil || service.issuer == nil || request.Declaration.ArtifactID == "" || request.Declaration.FirmwareVersion == "" || request.GrantID == "" || request.Container == "" || request.ExpiresAt.IsZero() {
		return UploadReceipt{}, ErrInvalidUploadRequest
	}
	now := service.now().UTC()
	if now.IsZero() || !request.ExpiresAt.After(now) || request.ExpiresAt.After(now.Add(5*time.Minute)) {
		return UploadReceipt{}, ErrInvalidUploadRequest
	}
	key, err := service.declarations.InitiateUpload(ctx, request.Declaration)
	if err != nil {
		return UploadReceipt{}, fmt.Errorf("reserve artifact upload: %w", err)
	}
	if key == "" {
		return UploadReceipt{}, fmt.Errorf("reserve artifact upload: %w", ErrInvalidUploadRequest)
	}
	capability, err := service.issuer.IssueUploadCapability(ctx, UploadCapabilityRequest{GrantID: request.GrantID, Container: request.Container, Key: key, Metadata: request.Declaration.Metadata, ExpiresAt: request.ExpiresAt.UTC()})
	if err != nil {
		return UploadReceipt{}, fmt.Errorf("issue direct upload capability: %w", err)
	}
	if capability.Authorization == "" || !capability.ExpiresAt.UTC().Equal(request.ExpiresAt.UTC()) || capability.Object.Provider() != artifact.ObjectProviderLocalV1 || capability.Object.Container() != request.Container || capability.Object.Key() != key || capability.Object.Size() != request.Declaration.Metadata.Size() || capability.Object.Digest() != request.Declaration.Metadata.Digest() {
		return UploadReceipt{}, fmt.Errorf("issue direct upload capability: %w", ErrInvalidUploadRequest)
	}
	return UploadReceipt{Authorization: capability.Authorization, Object: capability.Object, ExpiresAt: capability.ExpiresAt.UTC()}, nil
}
