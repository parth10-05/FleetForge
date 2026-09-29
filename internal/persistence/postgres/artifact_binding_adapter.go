package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

// ArtifactBindingAdapter implements the application binding port using the
// guarded serializable immutable-version transaction.
type ArtifactBindingAdapter struct{ Pool *pgxpool.Pool }

func (adapter ArtifactBindingAdapter) BindUploadedObject(ctx context.Context, artifactID string, object artifact.ObjectIdentity) error {
	if adapter.Pool == nil {
		return fmt.Errorf("artifact binding persistence is unavailable")
	}
	return BindUploadedArtifactObject(ctx, adapter.Pool, ArtifactBindingRequest{ArtifactID: artifactID, Object: object})
}
