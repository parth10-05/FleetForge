//go:build postgresintegration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func TestArtifactVerificationWorkerStreamsBoundVersionAndCompletesLease(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Now().UTC().Add(time.Second)
	const artifactID = "00000000-0000-0000-2621-000000000001"
	digest, err := value.ParseSHA256Digest("sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	if err != nil {
		t.Fatal(err)
	}
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/worker.bin", "version-worker-1", 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	store, err := localartifactstore.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(object, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ($1,'v1',$2,$3,'UPLOADING')", artifactID, digest.String(), object.Key()); err != nil {
		t.Fatal(err)
	}
	if err := persistence.BindUploadedArtifactObject(ctx, pool, persistence.ArtifactBindingRequest{ArtifactID: artifactID, Object: object}); err != nil {
		t.Fatal(err)
	}
	verification := persistence.ArtifactVerificationAdapter{Pool: pool}
	service := app.NewWithCompletion(nil, nil, store, nil, func() time.Time { return now }).WithVerification(verification, verification)
	work := persistence.ArtifactVerificationWorkAdapter{Pool: pool}
	worker := app.NewWorker(service, work, work, func() time.Time { return now })
	if err := worker.RunOnce(context.Background(), app.VerificationWorkClaimRequest{Owner: "verify-worker", LeaseToken: "00000000-0000-0000-2621-000000000004", Now: now, LeaseExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var state, workState, resultVersion string
	if err := pool.QueryRow(ctx, "SELECT a.state,w.delivery_state,r.object_version_id FROM firmware_artifacts a JOIN due_work w ON w.subject_id=a.id JOIN artifact_verification_results r ON r.artifact_id=a.id WHERE a.id=$1", artifactID).Scan(&state, &workState, &resultVersion); err != nil || state != "VERIFIED" || workState != "COMPLETED" || resultVersion != object.VersionID() {
		t.Fatalf("artifact=%q work=%q result=%q err=%v", state, workState, resultVersion, err)
	}
}
