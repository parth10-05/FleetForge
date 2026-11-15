//go:build postgresintegration

package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestUploadGrantReplayMetadataIsImmutable(t *testing.T) {
	ctx := context.Background()
	harness := New(t)
	if err := persistence.RunMigrations(ctx, integrationSettings(harness.ScopedDSN(t)), os.DirFS(filepath.Join("..", "..", "..", "db", "migrations"))); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	pool := harness.OpenPool(t)
	const artifactID = "00000000-0000-0000-2801-000000000001"
	mustExecute(t, pool, ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ('"+artifactID+"','v1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','objects/artifact.bin','INITIATED')")
	mustExecute(t, pool, ctx, "UPDATE firmware_artifacts SET upload_grant_id='grant-1',upload_grant_expires_at=now()+interval '5 minutes' WHERE id='"+artifactID+"'")
	if _, err := pool.Exec(ctx, "UPDATE firmware_artifacts SET upload_grant_expires_at=now()+interval '10 minutes' WHERE id='"+artifactID+"'"); err == nil || !strings.Contains(err.Error(), "upload grant metadata is immutable") {
		t.Fatalf("rewrite upload-grant expiry error = %v, want immutable metadata rejection", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state,upload_grant_expires_at) VALUES ('00000000-0000-0000-2801-000000000002','v2','sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','objects/second.bin','INITIATED',now())"); err == nil {
		t.Fatal("expiry without grant ID unexpectedly succeeded")
	}
}

func TestCampaignDraftIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	harness := New(t)
	if err := persistence.RunMigrations(ctx, integrationSettings(harness.ScopedDSN(t)), os.DirFS(filepath.Join("..", "..", "..", "db", "migrations"))); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	pool := harness.OpenPool(t)
	mustExecute(t, pool, ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ('00000000-0000-0000-2802-000000000001','v1','sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc','objects/campaign.bin','INITIATED')")
	mustExecute(t, pool, ctx, "INSERT INTO campaigns (id,artifact_id,state,policy_version_id) VALUES ('00000000-0000-0000-2802-000000000002','00000000-0000-0000-2802-000000000001','DRAFT','00000000-0000-0000-2802-000000000003')")
	mustExecute(t, pool, ctx, "INSERT INTO campaign_drafts (campaign_id,artifact_id,canonical_selector,policy_json,selection_seed,selection_algorithm,created_by) VALUES ('00000000-0000-0000-2802-000000000002','00000000-0000-0000-2802-000000000001','{}','{}','seed','selection/v1','user:operator')")
	if _, err := pool.Exec(ctx, "DELETE FROM campaign_drafts WHERE campaign_id='00000000-0000-0000-2802-000000000002'"); err == nil || !strings.Contains(err.Error(), "campaign drafts are immutable") {
		t.Fatalf("delete campaign draft error = %v, want immutable draft rejection", err)
	}
}
