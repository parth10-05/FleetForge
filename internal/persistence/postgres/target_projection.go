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

func ListTargetProjections(ctx context.Context, pool *pgxpool.Pool, afterID string, limit int) ([]TargetProjection, error) {
	if limit < 1 || limit > 101 {
		return nil, fmt.Errorf("invalid target projection limit")
	}
	if afterID != "" {
		if _, err := uuidParameter(afterID); err != nil {
			return nil, fmt.Errorf("invalid target projection cursor")
		}
	}
	rows, err := pool.Query(ctx, "SELECT id::text,administrative_state,revision,updated_at FROM targets WHERE (NULLIF($1,'')::uuid IS NULL OR id>NULLIF($1,'')::uuid) ORDER BY id LIMIT $2", afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list target projections: %w", err)
	}
	defer rows.Close()
	var projections []TargetProjection
	for rows.Next() {
		var p TargetProjection
		if err := rows.Scan(&p.ID, &p.State, &p.Revision, &p.AsOf); err != nil {
			return nil, fmt.Errorf("scan target projection: %w", err)
		}
		p.AsOf = p.AsOf.UTC()
		projections = append(projections, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate target projections: %w", err)
	}
	return projections, nil
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
