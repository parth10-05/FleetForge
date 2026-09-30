//go:build postgresintegration

package postgres

import (
	"strings"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func TestP02CleanupExpiresOnlyOldUnboundUploadingArtifacts(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	const expiredID = "00000000-0000-0000-2801-000000000001"
	const boundID = "00000000-0000-0000-2801-000000000002"
	const verifiedID = "00000000-0000-0000-2801-000000000003"
	for _, fixture := range []struct{ id, state, grant string }{{expiredID, "UPLOADING", "cleanup-grant-1"}, {boundID, "UPLOADING", "cleanup-grant-2"}, {verifiedID, "VERIFIED", "cleanup-grant-3"}} {
		if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,upload_grant_id,state,created_at) VALUES ($1,'v1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','pending',$2,$3,$4)", fixture.id, fixture.grant, fixture.state, now.Add(-25*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ($1,'local-versioned-v1','artifacts','artifacts/v1/bound.bin','version-1',1,'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','synthetic-sha256-v1','synthetic')", boundID); err != nil {
		t.Fatal(err)
	}
	expired, err := persistence.ExpireAbandonedArtifactUploads(ctx, pool, now.Add(-24*time.Hour), 25)
	if err != nil || len(expired) != 1 || expired[0].ArtifactID != expiredID || expired[0].GrantID != "cleanup-grant-1" {
		t.Fatalf("expired=%v err=%v", expired, err)
	}
	var intentGrant, intentState string
	if err := pool.QueryRow(ctx, "SELECT grant_id,state FROM artifact_cleanup_intents WHERE artifact_id=$1", expiredID).Scan(&intentGrant, &intentState); err != nil || intentGrant != "cleanup-grant-1" || intentState != "PENDING" {
		t.Fatalf("cleanup intent grant=%q state=%q err=%v", intentGrant, intentState, err)
	}
	claim, err := persistence.ClaimArtifactCleanupIntent(ctx, pool, "cleanup-worker", "00000000-0000-0000-2801-000000000099", now, now.Add(time.Minute))
	if err != nil || claim == nil || claim.ArtifactID != expiredID || claim.GrantID != "cleanup-grant-1" {
		t.Fatalf("cleanup claim=%+v err=%v", claim, err)
	}
	repaired, err := persistence.RepairExpiredArtifactCleanupIntents(ctx, pool, now.Add(2*time.Minute), 1)
	if err != nil || len(repaired) != 1 || repaired[0] != expiredID {
		t.Fatalf("repair=%v err=%v", repaired, err)
	}
	claim, err = persistence.ClaimArtifactCleanupIntent(ctx, pool, "recovery-worker", "00000000-0000-0000-2801-000000000098", now.Add(2*time.Minute), now.Add(3*time.Minute))
	if err != nil || claim == nil || claim.ArtifactID != expiredID || claim.GrantID != "cleanup-grant-1" {
		t.Fatalf("reclaimed cleanup intent=%+v err=%v", claim, err)
	}
	if err := persistence.CompleteArtifactCleanupIntent(ctx, pool, *claim, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("complete cleanup intent: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT state FROM artifact_cleanup_intents WHERE artifact_id=$1", expiredID).Scan(&intentState); err != nil || intentState != "COMPLETED" {
		t.Fatalf("completed intent state=%q err=%v", intentState, err)
	}
	for _, fixture := range []struct{ id, want string }{{expiredID, "EXPIRED"}, {boundID, "UPLOADING"}, {verifiedID, "VERIFIED"}} {
		var state string
		if err := pool.QueryRow(ctx, "SELECT state FROM firmware_artifacts WHERE id=$1", fixture.id).Scan(&state); err != nil || state != fixture.want {
			t.Fatalf("artifact=%s state=%s want=%s err=%v", fixture.id, state, fixture.want, err)
		}
	}
}

func TestP02CleanupWorkerDeletesOnlyClaimedGrantReceipt(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	const artifactID = "00000000-0000-0000-2802-000000000001"
	const grantID = "cleanup-component-grant"
	secret := []byte("01234567890123456789012345678901")
	store, err := localartifactstore.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claims := localartifactstore.UploadGrantClaims{Version: "fleetforge.upload-grant/v1", GrantID: grantID, ArtifactID: artifactID, Method: "PUT", Provider: artifact.ObjectProviderLocalV1, Container: "artifacts", Key: "artifacts/v1/cleanup.bin", VersionID: "cleanup-version-1", Size: 3, Digest: "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", MediaType: artifact.SyntheticFirmwareMediaType, ExpiresAt: now.Add(time.Minute)}
	grant, err := localartifactstore.IssueSignedGrant(secret, claims)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := store.PutSignedGrant(secret, grant, now, strings.NewReader("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,upload_grant_id,state,created_at) VALUES ($1,'v1',$2,'pending',$3,'UPLOADING',$4)", artifactID, claims.Digest, grantID, now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := persistence.ExpireAbandonedArtifactUploads(ctx, pool, now.Add(-24*time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	adapter := persistence.ArtifactCleanupIntentAdapter{Pool: pool}
	processed, err := app.RunCleanupOnce(ctx, adapter, adapter, store, store, "cleanup-worker", "00000000-0000-0000-2802-000000000099", now, now.Add(time.Minute))
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if _, err := store.Open(identity); err == nil {
		t.Fatal("claimed exact version remained after cleanup")
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT state FROM artifact_cleanup_intents WHERE artifact_id=$1", artifactID).Scan(&state); err != nil || state != "COMPLETED" {
		t.Fatalf("intent state=%q err=%v", state, err)
	}
}
