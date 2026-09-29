package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
)

// ArtifactUploadAdapter implements the application declaration port using the
// serializable P02 persistence transaction.
type ArtifactUploadAdapter struct{ Pool *pgxpool.Pool }

func (adapter ArtifactUploadAdapter) InitiateUpload(ctx context.Context, declaration app.UploadDeclaration) (string, error) {
	if adapter.Pool == nil {
		return "", fmt.Errorf("artifact upload persistence is unavailable")
	}
	return InitiateArtifactUpload(ctx, adapter.Pool, ArtifactUploadInitiation{ArtifactID: declaration.ArtifactID, FirmwareVersion: declaration.FirmwareVersion, Metadata: declaration.Metadata})
}
