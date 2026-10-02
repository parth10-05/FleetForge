package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrArtifactRetirementConflict = errors.New("artifact retirement conflict")

// RetireVerifiedArtifact makes an artifact ineligible for every new candidate.
// It intentionally retains object, verification, hold, and prior-candidate
// evidence for running or historical campaign audit.
func RetireVerifiedArtifact(ctx context.Context, pool *pgxpool.Pool, artifactID string) error {
	if artifactID == "" {
		return fmt.Errorf("invalid artifact ID")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin artifact retirement: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM firmware_artifacts WHERE id=$1::uuid FOR UPDATE`, artifactID).Scan(&state); err != nil {
		return fmt.Errorf("lock artifact retirement: %w", err)
	}
	if state != "VERIFIED" {
		return fmt.Errorf("%w: artifact state %s", ErrArtifactRetirementConflict, state)
	}
	if _, err := tx.Exec(ctx, `UPDATE firmware_artifacts SET state='RETIRED', revision=revision+1, updated_at=now() WHERE id=$1::uuid AND state='VERIFIED'`, artifactID); err != nil {
		return fmt.Errorf("retire artifact: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: retirement commit", ErrArtifactRetirementConflict)
		}
		return fmt.Errorf("commit artifact retirement: %w", err)
	}
	return nil
}
