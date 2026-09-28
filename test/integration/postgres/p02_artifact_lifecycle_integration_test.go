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

func TestP02ArtifactBindingAndVerificationPersistOneImmutableResult(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const artifactID = "00000000-0000-0000-2401-000000000001"
	digest, err := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/firmware.bin", "version-1", 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ($1,'v1',$2,'pending','UPLOADING')", artifactID, digest.String()); err != nil {
		t.Fatal(err)
	}
	if err := persistence.BindUploadedArtifactObject(ctx, pool, persistence.ArtifactBindingRequest{ArtifactID: artifactID, Object: object}); err != nil {
		t.Fatal(err)
	}
	if err := persistence.StartArtifactVerification(ctx, pool, artifactID); err != nil {
		t.Fatal(err)
	}
	if err := persistence.RecordArtifactVerification(ctx, pool, persistence.ArtifactVerificationRequest{ArtifactID: artifactID, VersionID: object.VersionID(), Object: object, Verified: true, ReasonCode: "digest_exact"}); err != nil {
		t.Fatal(err)
	}
	var state, resultDigest string
	var resultSize int64
	if err := pool.QueryRow(ctx, "SELECT a.state,r.verified_digest,r.verified_size FROM firmware_artifacts a JOIN artifact_verification_results r ON r.artifact_id=a.id WHERE a.id=$1", artifactID).Scan(&state, &resultDigest, &resultSize); err != nil || state != "VERIFIED" || resultDigest != digest.String() || resultSize != 3 {
		t.Fatalf("verification result state=%q digest=%q size=%d err=%v", state, resultDigest, resultSize, err)
	}
	if err := persistence.RecordArtifactVerification(ctx, pool, persistence.ArtifactVerificationRequest{ArtifactID: artifactID, VersionID: object.VersionID(), Object: object, Verified: true, ReasonCode: "digest_exact"}); !errors.Is(err, persistence.ErrArtifactBindingConflict) {
		t.Fatalf("replayed verification error=%v", err)
	}
}
