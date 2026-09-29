package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres/sqlcgen"
)

var ErrArtifactUploadConflict = errors.New("artifact upload declaration conflicts")

type ArtifactUploadInitiation struct {
	ArtifactID      string
	FirmwareVersion string
	Metadata        artifact.UploadMetadata
}

// InitiateArtifactUpload reserves the durable artifact declaration before the
// caller requests an exact scoped upload grant. It never accepts or proxies
// artifact bytes and the generated key never derives from DisplayName.
func InitiateArtifactUpload(ctx context.Context, pool *pgxpool.Pool, request ArtifactUploadInitiation) (string, error) {
	if pool == nil || !validFirmwareVersion(request.FirmwareVersion) {
		return "", fmt.Errorf("invalid artifact upload initiation")
	}
	id, err := uuidParameter(request.ArtifactID)
	if err != nil {
		return "", fmt.Errorf("artifact upload ID: %w", err)
	}
	artifactID, err := value.ParseArtifactID(request.ArtifactID)
	if err != nil {
		return "", fmt.Errorf("artifact upload ID: %w", err)
	}
	key, err := artifact.UploadObjectKey(artifactID)
	if err != nil {
		return "", fmt.Errorf("artifact upload key: %w", err)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return "", fmt.Errorf("begin artifact upload initiation: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	row, err := sqlcgen.New(tx).InitiateArtifactUpload(ctx, sqlcgen.InitiateArtifactUploadParams{ID: id, FirmwareVersion: request.FirmwareVersion, Sha256Digest: request.Metadata.Digest().String(), ObjectReference: key})
	if err == nil {
		if !row.Valid {
			return "", fmt.Errorf("initiate artifact upload: invalid returned ID")
		}
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit artifact upload initiation: %w", err)
		}
		return key, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("insert artifact upload declaration: %w", err)
	}
	var firmwareVersion, digest, objectReference, state string
	if err := tx.QueryRow(ctx, `SELECT firmware_version, sha256_digest, object_reference, state FROM firmware_artifacts WHERE id=$1 FOR SHARE`, id).Scan(&firmwareVersion, &digest, &objectReference, &state); err != nil {
		return "", fmt.Errorf("read artifact upload replay: %w", err)
	}
	if firmwareVersion != request.FirmwareVersion || digest != request.Metadata.Digest().String() || objectReference != key || state != "UPLOADING" {
		return "", ErrArtifactUploadConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit artifact upload replay: %w", err)
	}
	return key, nil
}

func validFirmwareVersion(raw string) bool {
	if len(raw) == 0 || len(raw) > 256 {
		return false
	}
	for _, character := range raw {
		if unicode.IsControl(character) {
			return false
		}
	}
	return strings.TrimSpace(raw) != ""
}
