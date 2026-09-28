//go:build postgresintegration

package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestP02ImmutableArtifactEvidenceRejectsRewriteAndDeletion(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	seedP02ArtifactEvidence(t, pool, ctx)
	assertSQLState(t, pool, ctx, "UPDATE artifact_object_versions SET object_key='rewritten.bin' WHERE artifact_id='00000000-0000-0000-2001-000000000001'", "55000")
	assertSQLState(t, pool, ctx, "DELETE FROM artifact_object_versions WHERE artifact_id='00000000-0000-0000-2001-000000000001'", "55000")
	assertSQLState(t, pool, ctx, "UPDATE artifact_verification_results SET state='REJECTED' WHERE artifact_id='00000000-0000-0000-2001-000000000001'", "55000")
}

func TestP02ArtifactHoldMayReleaseOnlyOnce(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	seedP02ArtifactEvidence(t, pool, ctx)
	if _, err := pool.Exec(ctx, "INSERT INTO campaigns (id,artifact_id,state,policy_version_id) VALUES ('00000000-0000-0000-2001-000000000002','00000000-0000-0000-2001-000000000001','DRAFT','00000000-0000-0000-2001-000000000003')"); err != nil { t.Fatal(err) }
	if _, err := pool.Exec(ctx, "INSERT INTO artifact_holds (artifact_id,campaign_id,object_version_id) VALUES ('00000000-0000-0000-2001-000000000001','00000000-0000-0000-2001-000000000002','v1')"); err != nil { t.Fatal(err) }
	if _, err := pool.Exec(ctx, "UPDATE artifact_holds SET released_at=now(),release_reason='campaign_finished' WHERE artifact_id='00000000-0000-0000-2001-000000000001' AND campaign_id='00000000-0000-0000-2001-000000000002'"); err != nil { t.Fatal(err) }
	assertSQLState(t, pool, ctx, "UPDATE artifact_holds SET release_reason='rewritten' WHERE artifact_id='00000000-0000-0000-2001-000000000001' AND campaign_id='00000000-0000-0000-2001-000000000002'", "55000")
	assertSQLState(t, pool, ctx, "DELETE FROM artifact_holds WHERE artifact_id='00000000-0000-0000-2001-000000000001' AND campaign_id='00000000-0000-0000-2001-000000000002'", "55000")
}

func seedP02ArtifactEvidence(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ('00000000-0000-0000-2001-000000000001','v1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','artifact-ref','VERIFIED')"); err != nil { t.Fatal(err) }
	if _, err := pool.Exec(ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ('00000000-0000-0000-2001-000000000001','local-versioned-v1','artifacts','firmware.bin','v1',1,'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','synthetic-sha256-v1','synthetic')"); err != nil { t.Fatal(err) }
	if _, err := pool.Exec(ctx, "INSERT INTO artifact_verification_results (artifact_id,object_version_id,policy_version,state,reason_code,verified_digest,verified_size) VALUES ('00000000-0000-0000-2001-000000000001','v1','synthetic-sha256-v1','VERIFIED','verified','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',1)"); err != nil { t.Fatal(err) }
}
