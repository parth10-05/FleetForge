//go:build postgresintegration

package postgres

import (
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestP02CleanupExpiresOnlyOldUnboundUploadingArtifacts(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	const expiredID = "00000000-0000-0000-2801-000000000001"
	const boundID = "00000000-0000-0000-2801-000000000002"
	const verifiedID = "00000000-0000-0000-2801-000000000003"
	for _, fixture := range []struct{ id, state string }{{expiredID, "UPLOADING"}, {boundID, "UPLOADING"}, {verifiedID, "VERIFIED"}} {
		if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state,created_at) VALUES ($1,'v1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','pending',$2,$3)", fixture.id, fixture.state, now.Add(-25*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ($1,'local-versioned-v1','artifacts','artifacts/v1/bound.bin','version-1',1,'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','synthetic-sha256-v1','synthetic')", boundID); err != nil {
		t.Fatal(err)
	}
	expired, err := persistence.ExpireAbandonedArtifactUploads(ctx, pool, now.Add(-24*time.Hour), 25)
	if err != nil || len(expired) != 1 || expired[0] != expiredID {
		t.Fatalf("expired=%v err=%v", expired, err)
	}
	for _, fixture := range []struct{ id, want string }{{expiredID, "EXPIRED"}, {boundID, "UPLOADING"}, {verifiedID, "VERIFIED"}} {
		var state string
		if err := pool.QueryRow(ctx, "SELECT state FROM firmware_artifacts WHERE id=$1", fixture.id).Scan(&state); err != nil || state != fixture.want {
			t.Fatalf("artifact=%s state=%s want=%s err=%v", fixture.id, state, fixture.want, err)
		}
	}
}
