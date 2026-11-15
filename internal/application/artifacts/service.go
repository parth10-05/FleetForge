// Package artifacts coordinates artifact declaration and direct-upload
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
	InitiateUpload(context.Context, UploadDeclaration, string) (string, error)
}

// UploadCapabilityRequest contains only the exact facts a direct capability
// may authorize. The issuer owns signing and provider version assignment.
type UploadCapabilityRequest struct {
	ArtifactID string
	GrantID    string
	Container  string
	Key        string
	// VersionID is supplied only when a durable command must regenerate an
	// identical capability. Empty retains provider-generated version identity.
	VersionID string
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

type UploadedObjectVerifier interface {
	VerifyObject(context.Context, artifact.ObjectIdentity) error
}

type UploadedObjectBinder interface {
	BindUploadedObject(context.Context, string, artifact.ObjectIdentity) error
}

type VerificationArtifactReader interface {
	LoadVerificationArtifact(context.Context, string) (artifact.ObjectIdentity, error)
}

type VerificationResultWriter interface {
	RecordVerification(context.Context, string, artifact.ObjectIdentity, bool, string) error
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
	declarations       UploadDeclarationWriter
	issuer             UploadCapabilityIssuer
	verifier           UploadedObjectVerifier
	binder             UploadedObjectBinder
	verificationReader VerificationArtifactReader
	verificationWriter VerificationResultWriter
	now                func() time.Time
}

func (service *Service) WithVerification(reader VerificationArtifactReader, writer VerificationResultWriter) *Service {
	service.verificationReader, service.verificationWriter = reader, writer
	return service
}

func NewWithCompletion(declarations UploadDeclarationWriter, issuer UploadCapabilityIssuer, verifier UploadedObjectVerifier, binder UploadedObjectBinder, now func() time.Time) *Service {
	service := New(declarations, issuer, now)
	service.verifier, service.binder = verifier, binder
	return service
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
	key, err := service.declarations.InitiateUpload(ctx, request.Declaration, request.GrantID)
	if err != nil {
		return UploadReceipt{}, fmt.Errorf("reserve artifact upload: %w", err)
	}
	if key == "" {
		return UploadReceipt{}, fmt.Errorf("reserve artifact upload: %w", ErrInvalidUploadRequest)
	}
	capability, err := service.issuer.IssueUploadCapability(ctx, UploadCapabilityRequest{ArtifactID: request.Declaration.ArtifactID, GrantID: request.GrantID, Container: request.Container, Key: key, Metadata: request.Declaration.Metadata, ExpiresAt: request.ExpiresAt.UTC()})
	if err != nil {
		return UploadReceipt{}, fmt.Errorf("issue direct upload capability: %w", err)
	}
	if capability.Authorization == "" || !capability.ExpiresAt.UTC().Equal(request.ExpiresAt.UTC()) || capability.Object.Provider() != artifact.ObjectProviderLocalV1 || capability.Object.Container() != request.Container || capability.Object.Key() != key || capability.Object.Size() != request.Declaration.Metadata.Size() || capability.Object.Digest() != request.Declaration.Metadata.Digest() {
		return UploadReceipt{}, fmt.Errorf("issue direct upload capability: %w", ErrInvalidUploadRequest)
	}
	return UploadReceipt{Authorization: capability.Authorization, Object: capability.Object, ExpiresAt: capability.ExpiresAt.UTC()}, nil
}

// CompleteUpload verifies bytes through the data-plane adapter before it asks
// persistence to bind the immutable receipt. There is no storage I/O inside
// the binding transaction.
func (service *Service) CompleteUpload(ctx context.Context, artifactID string, object artifact.ObjectIdentity) error {
	if service == nil || service.verifier == nil || service.binder == nil || artifactID == "" || object.Provider() != artifact.ObjectProviderLocalV1 || object.VersionID() == "" {
		return ErrInvalidUploadRequest
	}
	if err := service.verifier.VerifyObject(ctx, object); err != nil {
		return fmt.Errorf("verify uploaded object: %w", err)
	}
	if err := service.binder.BindUploadedObject(ctx, artifactID, object); err != nil {
		return fmt.Errorf("bind verified uploaded object: %w", err)
	}
	return nil
}

// VerifyBoundArtifact loads the already-bound immutable version, performs
// storage I/O outside persistence, and records exactly one stable outcome.
func (service *Service) VerifyBoundArtifact(ctx context.Context, artifactID string) error {
	if service == nil || service.verificationReader == nil || service.verificationWriter == nil || service.verifier == nil || artifactID == "" {
		return ErrInvalidUploadRequest
	}
	object, err := service.verificationReader.LoadVerificationArtifact(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("load verification artifact: %w", err)
	}
	if err := service.verifier.VerifyObject(ctx, object); err != nil {
		if recordErr := service.verificationWriter.RecordVerification(ctx, artifactID, object, false, "artifact.verification.failed"); recordErr != nil {
			return fmt.Errorf("record rejected artifact verification: %w", recordErr)
		}
		return nil
	}
	if err := service.verificationWriter.RecordVerification(ctx, artifactID, object, true, "digest_exact"); err != nil {
		return fmt.Errorf("record verified artifact: %w", err)
	}
	return nil
}
