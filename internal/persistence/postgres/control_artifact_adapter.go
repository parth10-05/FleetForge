package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/control"
)

// ControlArtifactAdapter is the PostgreSQL artifact-read port for the public
// control API. It performs no object-store access.
type ControlArtifactAdapter struct{ Pool *pgxpool.Pool }

func (adapter ControlArtifactAdapter) GetArtifact(ctx context.Context, artifactID string) (control.ArtifactProjection, error) {
	if adapter.Pool == nil {
		return control.ArtifactProjection{}, fmt.Errorf("artifact projection persistence is unavailable")
	}
	projection, err := GetArtifactProjection(ctx, adapter.Pool, artifactID)
	if errors.Is(err, ErrArtifactProjectionNotFound) {
		return control.ArtifactProjection{}, control.ErrArtifactNotFound
	}
	if err != nil {
		return control.ArtifactProjection{}, err
	}
	return control.ArtifactProjection{ArtifactID: projection.ID, State: projection.State, AsOf: projection.AsOf}, nil
}

var _ control.ArtifactProjectionReader = ControlArtifactAdapter{}
