package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres/sqlcgen"
)

const maxArtifactCleanupBatch = 25

// ExpiredUpload is the only authority an external cleanup caller receives
// after the expiration transaction commits. It contains no mutable key/path
// and must be resolved through a grant-scoped receipt before exact deletion.
type ExpiredUpload struct {
	ArtifactID string
	GrantID    string
}

// ExpireAbandonedArtifactUploads expires only unbound UPLOADING artifacts
// older than cutoff. It never deletes bytes and never affects verified or
// campaign-held artifact evidence; physical deletion remains a separate exact
// object-store operation after this workflow decision has committed.
func ExpireAbandonedArtifactUploads(ctx context.Context, pool *pgxpool.Pool, cutoff time.Time, limit int) ([]ExpiredUpload, error) {
	if pool == nil || cutoff.IsZero() || limit <= 0 || limit > maxArtifactCleanupBatch {
		return nil, fmt.Errorf("invalid artifact cleanup request")
	}
	rows, err := sqlcgen.New(pool).ExpireAbandonedArtifactUploads(ctx, sqlcgen.ExpireAbandonedArtifactUploadsParams{Column1: pgtype.Timestamptz{Time: cutoff.UTC(), Valid: true}, Limit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("expire abandoned artifact uploads: %w", err)
	}
	result := make([]ExpiredUpload, 0, len(rows))
	for _, row := range rows {
		id, err := sqlcUUIDText(row.ArtifactID)
		if err != nil {
			return nil, err
		}
		if row.GrantID == "" {
			return nil, fmt.Errorf("expired upload missing durable grant authority")
		}
		result = append(result, ExpiredUpload{ArtifactID: id, GrantID: row.GrantID})
	}
	return result, nil
}
