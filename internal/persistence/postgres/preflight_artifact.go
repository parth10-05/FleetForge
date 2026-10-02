package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

type PreflightArtifact struct {
	ID     value.ArtifactID
	State  artifact.State
	Family value.LabelValue
	Object artifact.ObjectIdentity
}

// LoadPreflightArtifact reads only the typed immutable facts needed by the
// application evaluator. PersistPreflightCandidate repeats all eligibility
// checks under locks before it commits a frozen candidate.
func LoadPreflightArtifact(ctx context.Context, pool *pgxpool.Pool, artifactID value.ArtifactID) (PreflightArtifact, error) {
	if pool == nil || artifactID.IsZero() {
		return PreflightArtifact{}, fmt.Errorf("invalid preflight artifact request")
	}
	var rawID, stateRaw, familyRaw, provider, container, key, versionID, digestRaw string
	var size int64
	err := pool.QueryRow(ctx, `SELECT a.id::text, a.state, o.declared_target_family, o.provider, o.container, o.object_key, o.version_id, o.object_size, o.sha256_digest FROM firmware_artifacts a JOIN artifact_object_versions o ON o.artifact_id=a.id WHERE a.id=$1::uuid`, artifactID.String()).Scan(&rawID, &stateRaw, &familyRaw, &provider, &container, &key, &versionID, &size, &digestRaw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PreflightArtifact{}, fmt.Errorf("preflight artifact %s not found", artifactID.String())
		}
		return PreflightArtifact{}, fmt.Errorf("load preflight artifact: %w", err)
	}
	parsedID, err := value.ParseArtifactID(rawID)
	if err != nil {
		return PreflightArtifact{}, fmt.Errorf("map preflight artifact ID: %w", err)
	}
	state, err := artifact.ParseState(stateRaw)
	if err != nil {
		return PreflightArtifact{}, fmt.Errorf("map preflight artifact state: %w", err)
	}
	family, err := value.ParseLabelValue(familyRaw)
	if err != nil {
		return PreflightArtifact{}, fmt.Errorf("map preflight artifact family: %w", err)
	}
	digest, err := value.ParseSHA256Digest(digestRaw)
	if err != nil {
		return PreflightArtifact{}, fmt.Errorf("map preflight artifact digest: %w", err)
	}
	object, err := artifact.NewObjectIdentity(provider, container, key, versionID, size, digest)
	if err != nil {
		return PreflightArtifact{}, fmt.Errorf("map preflight object identity: %w", err)
	}
	return PreflightArtifact{ID: parsedID, State: state, Family: family, Object: object}, nil
}
