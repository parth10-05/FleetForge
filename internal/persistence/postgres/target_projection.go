package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTargetProjectionNotFound = errors.New("target projection not found")

// TargetProjection is the safe, bounded read model for one registered target.
type TargetProjection struct {
	ID, State string
	Revision  int64
	AsOf      time.Time
}

func GetTargetProjection(ctx context.Context, pool *pgxpool.Pool, targetID string) (TargetProjection, error) {
	if _, err := uuidParameter(targetID); err != nil {
		return TargetProjection{}, fmt.Errorf("invalid target projection ID")
	}
	var projection TargetProjection
	if err := pool.QueryRow(ctx, "SELECT id::text,administrative_state,revision,updated_at FROM targets WHERE id=$1::uuid", targetID).Scan(&projection.ID, &projection.State, &projection.Revision, &projection.AsOf); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TargetProjection{}, ErrTargetProjectionNotFound
		}
		return TargetProjection{}, fmt.Errorf("load target projection: %w", err)
	}
	projection.AsOf = projection.AsOf.UTC()
	return projection, nil
}
