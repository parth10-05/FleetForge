package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
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
// version, moves the artifact to VERIFYING, and writes the verification work
// that makes the uploaded artifact consumable by the orchestrator worker.
func BindUploadedArtifactObject(ctx context.Context, pool *pgxpool.Pool, request ArtifactBindingRequest) error {
	if request.ArtifactID == "" || request.Object.VersionID() == "" {
		return fmt.Errorf("invalid artifact binding request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin artifact binding: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state, declaredDigest, declaredKey string
	var revision int64
	if err := tx.QueryRow(ctx, "SELECT state,revision,sha256_digest,object_reference FROM firmware_artifacts WHERE id=$1::uuid FOR UPDATE", request.ArtifactID).Scan(&state, &revision, &declaredDigest, &declaredKey); err != nil {
		return fmt.Errorf("lock artifact: %w", err)
	}
	if state != "UPLOADING" && state != "VERIFYING" {
		return fmt.Errorf("%w: artifact state %s", ErrArtifactBindingConflict, state)
	}
	if request.Object.Provider() != artifact.ObjectProviderLocalV1 || request.Object.Key() != declaredKey || request.Object.Digest().String() != declaredDigest {
		return ErrArtifactBindingConflict
	}
	var existingVersion string
	err = tx.QueryRow(ctx, "SELECT version_id FROM artifact_object_versions WHERE artifact_id=$1::uuid", request.ArtifactID).Scan(&existingVersion)
	if err == nil {
		if existingVersion != request.Object.VersionID() {
			return ErrArtifactBindingConflict
		}
		if state != "VERIFYING" {
			return ErrArtifactBindingConflict
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM due_work WHERE subject_type='ARTIFACT' AND subject_id=$1::uuid AND work_type='VERIFY_ARTIFACT' AND delivery_state IN ('PENDING','CLAIMED'))`, request.ArtifactID).Scan(&active); err != nil {
			return fmt.Errorf("read active artifact verification work: %w", err)
		}
		if !active {
			return fmt.Errorf("%w: verifying artifact has no active work", ErrArtifactBindingConflict)
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
	var expectedRevision int64
	if err := tx.QueryRow(ctx, "UPDATE firmware_artifacts SET state='VERIFYING',revision=revision+1,updated_at=now() WHERE id=$1::uuid AND state='UPLOADING' RETURNING revision", request.ArtifactID).Scan(&expectedRevision); err != nil {
		return fmt.Errorf("start artifact verification: %w", err)
	}
	if expectedRevision != revision+1 {
		return fmt.Errorf("%w: invalid verification revision", ErrArtifactBindingConflict)
	}
	artifactUUID, err := uuid.Parse(request.ArtifactID)
	if err != nil {
		return fmt.Errorf("artifact verification identity: %w", err)
	}
	workID := uuid.NewSHA1(artifactUUID, []byte("fleetforge.artifact-verification/v1:work")).String()
	commandID := uuid.NewSHA1(artifactUUID, []byte("fleetforge.artifact-verification/v1:command")).String()
	var created string
	err = tx.QueryRow(ctx, `INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision, expected_command_epoch) VALUES ($1,$2,'ARTIFACT',$3::uuid,'VERIFY_ARTIFACT',1,now(),$4,NULL) ON CONFLICT (subject_id, work_type, expected_revision) WHERE subject_type='ARTIFACT' AND work_type='VERIFY_ARTIFACT' AND delivery_state IN ('PENDING','CLAIMED') DO NOTHING RETURNING id`, workID, commandID, request.ArtifactID, expectedRevision).Scan(&created)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: active artifact verification work already exists", ErrArtifactBindingConflict)
	}
	if err != nil {
		return fmt.Errorf("insert artifact verification work: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: binding commit", ErrArtifactBindingConflict)
		}
		return fmt.Errorf("commit artifact binding: %w", err)
	}
	return nil
}
