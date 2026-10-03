//go:build postgresintegration

package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestArtifactUploadInitiationReservesGeneratedKeyIdempotently(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	digest, err := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := artifact.NewUploadMetadata("firmware.bin", artifact.SyntheticFirmwareMediaType, 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	request := persistence.ArtifactUploadInitiation{ArtifactID: "00000000-0000-0000-2901-000000000001", FirmwareVersion: "v1", Metadata: metadata, GrantID: "grant-initiation-1"}
	key, err := persistence.InitiateArtifactUpload(ctx, pool, request)
	if err != nil || key != "artifacts/v1/00000000-0000-0000-2901-000000000001.bin" {
		t.Fatalf("key=%q err=%v", key, err)
	}
	replayKey, err := persistence.InitiateArtifactUpload(ctx, pool, request)
	if err != nil || replayKey != key {
		t.Fatalf("replay key=%q err=%v", replayKey, err)
	}
	differentDigest, _ := value.ParseSHA256Digest("sha256:" + strings.Repeat("b", 64))
	differentMetadata, _ := artifact.NewUploadMetadata("firmware.bin", artifact.SyntheticFirmwareMediaType, 3, differentDigest)
	request.Metadata = differentMetadata
	if _, err := persistence.InitiateArtifactUpload(ctx, pool, request); !errors.Is(err, persistence.ErrArtifactUploadConflict) {
		t.Fatalf("conflicting declaration error=%v", err)
	}
}
