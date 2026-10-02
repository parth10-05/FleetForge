//go:build postgresintegration

package postgres

import (
	"errors"
	"strings"
	"testing"
	"time"

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
	const key = "artifacts/v1/firmware.bin"
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", key, "version-1", 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ($1,'v1',$2,$3,'UPLOADING')", artifactID, digest.String(), key); err != nil {
		t.Fatal(err)
	}
	wrongKey, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/other.bin", "version-1", 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.BindUploadedArtifactObject(ctx, pool, persistence.ArtifactBindingRequest{ArtifactID: artifactID, Object: wrongKey}); !errors.Is(err, persistence.ErrArtifactBindingConflict) {
		t.Fatalf("wrong key binding error=%v", err)
	}
	wrongDigest, err := value.ParseSHA256Digest("sha256:" + strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	wrongContent, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", key, "version-1", 3, wrongDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.BindUploadedArtifactObject(ctx, pool, persistence.ArtifactBindingRequest{ArtifactID: artifactID, Object: wrongContent}); !errors.Is(err, persistence.ErrArtifactBindingConflict) {
		t.Fatalf("wrong digest binding error=%v", err)
	}
	if err := persistence.BindUploadedArtifactObject(ctx, pool, persistence.ArtifactBindingRequest{ArtifactID: artifactID, Object: object}); err != nil {
		t.Fatal(err)
	}
	start := persistence.ArtifactVerificationStartRequest{ArtifactID: artifactID, WorkID: "00000000-0000-0000-2401-000000000002", CommandID: "00000000-0000-0000-2401-000000000003", NotBefore: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}
	scheduled, err := persistence.StartAndScheduleArtifactVerification(ctx, pool, start)
	if err != nil || !scheduled {
		t.Fatalf("start scheduled=%v err=%v", scheduled, err)
	}
	repeated, err := persistence.StartAndScheduleArtifactVerification(ctx, pool, start)
	if err != nil || repeated {
		t.Fatalf("repeat start scheduled=%v err=%v", repeated, err)
	}
	wrongObject, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/substituted.bin", object.VersionID(), object.Size(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.RecordArtifactVerification(ctx, pool, persistence.ArtifactVerificationRequest{ArtifactID: artifactID, VersionID: object.VersionID(), Object: wrongObject, Verified: true, ReasonCode: "digest_exact"}); !errors.Is(err, persistence.ErrArtifactBindingConflict) {
		t.Fatalf("substituted tuple error=%v", err)
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
