//go:build postgresintegration

package postgres

import (
	"context"
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestArtifactVerificationSchedulingDeduplicatesActiveRevision(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const artifactID = "00000000-0000-0000-2501-000000000001"
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state,revision) VALUES ($1,'v1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','pending','UPLOADED',2)", artifactID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	first, err := persistence.ScheduleArtifactVerification(ctx, pool, persistence.ArtifactVerificationScheduleRequest{WorkID: "00000000-0000-0000-2501-000000000002", CommandID: "00000000-0000-0000-2501-000000000003", ArtifactID: artifactID, ArtifactRevision: 2, NotBefore: now})
	if err != nil || !first {
		t.Fatalf("first schedule=%v err=%v", first, err)
	}
	second, err := persistence.ScheduleArtifactVerification(ctx, pool, persistence.ArtifactVerificationScheduleRequest{WorkID: "00000000-0000-0000-2501-000000000004", CommandID: "00000000-0000-0000-2501-000000000005", ArtifactID: artifactID, ArtifactRevision: 2, NotBefore: now})
	if err != nil || second {
		t.Fatalf("duplicate schedule=%v err=%v", second, err)
	}
	third, err := persistence.ScheduleArtifactVerification(context.Background(), pool, persistence.ArtifactVerificationScheduleRequest{WorkID: "00000000-0000-0000-2501-000000000006", CommandID: "00000000-0000-0000-2501-000000000007", ArtifactID: artifactID, ArtifactRevision: 3, NotBefore: now})
	if err != nil || !third {
		t.Fatalf("new revision schedule=%v err=%v", third, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE subject_id=$1 AND subject_type='ARTIFACT' AND work_type='VERIFY_ARTIFACT'", artifactID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("verification work count=%d err=%v", count, err)
	}
}
