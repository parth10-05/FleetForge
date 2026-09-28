package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

var ErrArtifactBindingConflict = errors.New("artifact object binding conflict")

type ArtifactBindingRequest struct {
	ArtifactID string
	Object     artifact.ObjectIdentity
}

// BindUploadedArtifactObject atomically attaches exactly one provider-assigned
// version to an UPLOADING artifact. It never replaces an existing identity.
func BindUploadedArtifactObject(ctx context.Context, pool *pgxpool.Pool, request ArtifactBindingRequest) error {
	if request.ArtifactID == "" || request.Object.VersionID() == "" {
		return fmt.Errorf("invalid artifact binding request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin artifact binding: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM firmware_artifacts WHERE id=$1::uuid FOR UPDATE", request.ArtifactID).Scan(&state); err != nil {
		return fmt.Errorf("lock artifact: %w", err)
	}
	if state != "UPLOADING" && state != "UPLOADED" {
		return fmt.Errorf("%w: artifact state %s", ErrArtifactBindingConflict, state)
	}
	var existingVersion string
	err = tx.QueryRow(ctx, "SELECT version_id FROM artifact_object_versions WHERE artifact_id=$1::uuid", request.ArtifactID).Scan(&existingVersion)
	if err == nil {
		if existingVersion != request.Object.VersionID() {
			return ErrArtifactBindingConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit repeated artifact binding: %w", err)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read artifact binding: %w", err)
	}
	if state != "UPLOADING" {
		return ErrArtifactBindingConflict
	}
	if _, err := tx.Exec(ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9)", request.ArtifactID, request.Object.Provider(), request.Object.Container(), request.Object.Key(), request.Object.VersionID(), request.Object.Size(), request.Object.Digest().String(), artifact.VerificationPolicySyntheticSHA256V1, "synthetic"); err != nil {
		return fmt.Errorf("insert immutable artifact object version: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE firmware_artifacts SET state='UPLOADED',revision=revision+1,updated_at=now() WHERE id=$1::uuid AND state='UPLOADING'", request.ArtifactID); err != nil {
		return fmt.Errorf("transition artifact to uploaded: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: binding commit", ErrArtifactBindingConflict)
		}
		return fmt.Errorf("commit artifact binding: %w", err)
	}
	return nil
}
