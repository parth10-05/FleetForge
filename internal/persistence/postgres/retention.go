package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CleanupExpiredObservations removes at most limit old observation rows. It is
// intentionally the only destructive P03-14 operation and cannot affect audit,
// replay, work, enrollment, admission, reservation, or capacity evidence.
func CleanupExpiredObservations(ctx context.Context, pool *pgxpool.Pool, cutoff time.Time, limit int) (int64, error) {
	if cutoff.IsZero() || limit <= 0 || limit > 1000 {
		return 0, fmt.Errorf("invalid observation cleanup request")
	}
	tag, err := pool.Exec(ctx, `WITH eligible AS (SELECT id FROM device_observations WHERE received_at < $1 ORDER BY received_at,id LIMIT $2 FOR UPDATE SKIP LOCKED) DELETE FROM device_observations d USING eligible e WHERE d.id=e.id`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired observations: %w", err)
	}
	return tag.RowsAffected(), nil
}
