package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// ArtifactVerificationAdapter provides the application worker with the exact
// persisted version and result writer. It performs no storage I/O.
type ArtifactVerificationAdapter struct{ Pool *pgxpool.Pool }

func (adapter ArtifactVerificationAdapter) LoadVerificationArtifact(ctx context.Context, artifactID string) (artifact.ObjectIdentity, error) {
	if adapter.Pool == nil {
		return artifact.ObjectIdentity{}, fmt.Errorf("artifact verification persistence is unavailable")
	}
	id, err := value.ParseArtifactID(artifactID)
	if err != nil {
		return artifact.ObjectIdentity{}, fmt.Errorf("parse verification artifact ID: %w", err)
	}
	record, err := LoadPreflightArtifact(ctx, adapter.Pool, id)
	if err != nil {
		return artifact.ObjectIdentity{}, err
	}
	if record.State != artifact.StateVerifying {
		return artifact.ObjectIdentity{}, fmt.Errorf("artifact verification state is %s", record.State)
	}
	return record.Object, nil
}

func (adapter ArtifactVerificationAdapter) RecordVerification(ctx context.Context, artifactID string, object artifact.ObjectIdentity, verified bool, reason string) error {
	if adapter.Pool == nil {
		return fmt.Errorf("artifact verification persistence is unavailable")
	}
	return RecordArtifactVerification(ctx, adapter.Pool, ArtifactVerificationRequest{ArtifactID: artifactID, VersionID: object.VersionID(), Object: object, Verified: verified, ReasonCode: reason})
}
