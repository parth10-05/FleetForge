package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

var ErrArtifactNotFound = errors.New("artifact projection not found")

type ArtifactProjection struct {
	ArtifactID string
	State      string
	AsOf       time.Time
}

type ArtifactProjectionReader interface {
	GetArtifact(context.Context, string) (ArtifactProjection, error)
}

type ArtifactService struct {
	store ArtifactProjectionReader
	now   func() time.Time
}

func NewArtifactService(store ArtifactProjectionReader, now func() time.Time) *ArtifactService {
	if now == nil {
		now = time.Now
	}
	return &ArtifactService{store: store, now: now}
}

func (service *ArtifactService) GetArtifact(ctx context.Context, principal access.Principal, artifactID string) (ArtifactProjection, error) {
	if service == nil || service.store == nil || artifactID == "" {
		return ArtifactProjection{}, fmt.Errorf("artifact projection persistence is unavailable")
	}
	if err := access.Authorize(principal, access.PermissionReadArtifacts, service.now()); err != nil {
		return ArtifactProjection{}, err
	}
	return service.store.GetArtifact(ctx, artifactID)
}
