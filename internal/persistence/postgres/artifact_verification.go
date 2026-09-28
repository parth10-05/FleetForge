package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

type ArtifactVerificationRequest struct {
	ArtifactID, VersionID, ReasonCode string
	Verified                          bool
	Object                            artifact.ObjectIdentity
}

// StartArtifactVerification atomically claims the immutable uploaded object
// for verification. Storage I/O must happen only after this transaction commits.
func StartArtifactVerification(ctx context.Context, pool *pgxpool.Pool, artifactID string) error {
	if artifactID == "" {
		return fmt.Errorf("invalid artifact ID")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin artifact verification: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM firmware_artifacts WHERE id=$1::uuid FOR UPDATE`, artifactID).Scan(&state); err != nil {
		return fmt.Errorf("lock artifact verification: %w", err)
	}
	if state != "UPLOADED" {
		return fmt.Errorf("%w: artifact state %s", ErrArtifactBindingConflict, state)
	}
	var versionID string
	if err := tx.QueryRow(ctx, `SELECT version_id FROM artifact_object_versions WHERE artifact_id=$1::uuid`, artifactID).Scan(&versionID); err != nil {
		return fmt.Errorf("read artifact object version: %w", err)
	}
	if versionID == "" {
		return fmt.Errorf("%w: missing immutable object version", ErrArtifactBindingConflict)
	}
	if _, err := tx.Exec(ctx, `UPDATE firmware_artifacts SET state='VERIFYING', revision=revision+1, updated_at=now() WHERE id=$1::uuid AND state='UPLOADED'`, artifactID); err != nil {
		return fmt.Errorf("start artifact verification: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: verification start commit", ErrArtifactBindingConflict)
		}
		return fmt.Errorf("commit artifact verification start: %w", err)
	}
	return nil
}

// RecordArtifactVerification persists one stable exact-version result after
// the worker has performed storage I/O. It never streams or opens bytes here.
func RecordArtifactVerification(ctx context.Context, pool *pgxpool.Pool, request ArtifactVerificationRequest) error {
	if request.ArtifactID == "" || request.VersionID == "" || request.ReasonCode == "" || request.Object.VersionID() != request.VersionID {
		return fmt.Errorf("invalid artifact verification request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin artifact verification: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state, provider, container, key, boundVersion, boundDigest string
	var boundSize int64
	if err := tx.QueryRow(ctx, `SELECT a.state, o.provider, o.container, o.object_key, o.version_id, o.object_size, o.sha256_digest FROM firmware_artifacts a JOIN artifact_object_versions o ON o.artifact_id = a.id WHERE a.id = $1::uuid FOR UPDATE`, request.ArtifactID).Scan(&state, &provider, &container, &key, &boundVersion, &boundSize, &boundDigest); err != nil {
		return fmt.Errorf("lock artifact verification: %w", err)
	}
	if boundVersion != request.VersionID || provider != request.Object.Provider() || container != request.Object.Container() || key != request.Object.Key() || boundSize != request.Object.Size() || boundDigest != request.Object.Digest().String() {
		return ErrArtifactBindingConflict
	}
	if state != "VERIFYING" {
		return fmt.Errorf("%w: artifact state %s", ErrArtifactBindingConflict, state)
	}
	next := "REJECTED"
	if request.Verified {
		next = "VERIFIED"
	}
	var digest any
	var size any
	if request.Verified {
		digest, size = request.Object.Digest().String(), request.Object.Size()
	}
	if _, err := tx.Exec(ctx, `INSERT INTO artifact_verification_results (artifact_id, object_version_id, policy_version, state, reason_code, verified_digest, verified_size) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)`, request.ArtifactID, request.VersionID, artifact.VerificationPolicySyntheticSHA256V1, next, request.ReasonCode, digest, size); err != nil {
		return fmt.Errorf("insert verification result: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE firmware_artifacts SET state=$2, revision=revision+1, updated_at=now() WHERE id=$1::uuid AND state='VERIFYING'`, request.ArtifactID, next); err != nil {
		return fmt.Errorf("transition artifact verification: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: verification commit", ErrArtifactBindingConflict)
		}
		return fmt.Errorf("commit artifact verification: %w", err)
	}
	return nil
}
