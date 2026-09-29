package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ArtifactVerificationStartRequest struct {
	ArtifactID, WorkID, CommandID string
	NotBefore                     time.Time
}

// StartAndScheduleArtifactVerification atomically transitions one uploaded
// immutable object to VERIFYING and writes its durable worker intent. Storage
// reads occur only after a worker later claims that committed due-work row.
func StartAndScheduleArtifactVerification(ctx context.Context, pool *pgxpool.Pool, request ArtifactVerificationStartRequest) (scheduled bool, err error) {
	if pool == nil || request.NotBefore.IsZero() {
		return false, fmt.Errorf("invalid artifact verification start request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, fmt.Errorf("begin artifact verification start: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state string
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT state, revision FROM firmware_artifacts WHERE id=$1::uuid FOR UPDATE`, request.ArtifactID).Scan(&state, &revision); err != nil {
		return false, fmt.Errorf("lock artifact verification start: %w", err)
	}
	if state == "VERIFYING" {
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM due_work WHERE subject_type='ARTIFACT' AND subject_id=$1::uuid AND work_type='VERIFY_ARTIFACT' AND delivery_state IN ('PENDING','CLAIMED'))`, request.ArtifactID).Scan(&active); err != nil {
			return false, fmt.Errorf("read active verification work: %w", err)
		}
		if active {
			if err := tx.Commit(ctx); err != nil {
				return false, fmt.Errorf("commit repeated verification start: %w", err)
			}
			return false, nil
		}
		return false, fmt.Errorf("%w: verifying artifact has no active work", ErrArtifactBindingConflict)
	}
	if state != "UPLOADED" || revision < 0 {
		return false, fmt.Errorf("%w: artifact state %s", ErrArtifactBindingConflict, state)
	}
	var versionID string
	if err := tx.QueryRow(ctx, `SELECT version_id FROM artifact_object_versions WHERE artifact_id=$1::uuid FOR SHARE`, request.ArtifactID).Scan(&versionID); err != nil || versionID == "" {
		return false, fmt.Errorf("%w: artifact has no bound immutable object", ErrArtifactBindingConflict)
	}
	workID, err := uuidParameter(request.WorkID)
	if err != nil {
		return false, fmt.Errorf("artifact verification work ID: %w", err)
	}
	commandID, err := uuidParameter(request.CommandID)
	if err != nil {
		return false, fmt.Errorf("artifact verification command ID: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE firmware_artifacts SET state='VERIFYING', revision=revision+1, updated_at=now() WHERE id=$1::uuid AND state='UPLOADED'`, request.ArtifactID); err != nil {
		return false, fmt.Errorf("start artifact verification: %w", err)
	}
	var created string
	err = tx.QueryRow(ctx, `INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision, expected_command_epoch) VALUES ($1,$2,'ARTIFACT',$3::uuid,'VERIFY_ARTIFACT',1,$4,$5,NULL) ON CONFLICT (subject_id, work_type, expected_revision) WHERE subject_type='ARTIFACT' AND work_type='VERIFY_ARTIFACT' AND delivery_state IN ('PENDING','CLAIMED') DO NOTHING RETURNING id`, workID, commandID, request.ArtifactID, request.NotBefore.UTC(), revision+1).Scan(&created)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("%w: active artifact verification work already exists", ErrArtifactBindingConflict)
	}
	if err != nil {
		return false, fmt.Errorf("insert artifact verification work: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return false, fmt.Errorf("%w: verification start commit", ErrArtifactBindingConflict)
		}
		return false, fmt.Errorf("commit artifact verification start: %w", err)
	}
	return true, nil
}
