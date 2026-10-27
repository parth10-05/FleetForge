package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrArtifactProjectionNotFound = errors.New("artifact projection not found")

type ArtifactProjection struct {
	ID, State string
	AsOf      time.Time
}

func GetArtifactProjection(ctx context.Context, pool *pgxpool.Pool, artifactID string) (ArtifactProjection, error) {
	if _, err := uuidParameter(artifactID); err != nil {
		return ArtifactProjection{}, fmt.Errorf("invalid artifact projection ID")
	}
	var projection ArtifactProjection
	if err := pool.QueryRow(ctx, "SELECT id::text,state,updated_at FROM firmware_artifacts WHERE id=$1::uuid", artifactID).Scan(&projection.ID, &projection.State, &projection.AsOf); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ArtifactProjection{}, ErrArtifactProjectionNotFound
		}
		return ArtifactProjection{}, fmt.Errorf("load artifact projection: %w", err)
	}
	projection.AsOf = projection.AsOf.UTC()
	return projection, nil
}
