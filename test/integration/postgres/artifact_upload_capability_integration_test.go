//go:build postgresintegration

package postgres

import (
	"strings"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactissuer"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func TestArtifactUploadCapabilityReservesDatabaseKeyBeforeSigning(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	digest, err := value.ParseSHA256Digest("sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := artifact.NewUploadMetadata("firmware.bin", artifact.SyntheticFirmwareMediaType, 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	secret := []byte("01234567890123456789012345678901")
	store, err := localartifactstore.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := app.NewWithCompletion(persistence.ArtifactUploadAdapter{Pool: pool}, localartifactissuer.New(secret, nil), store, persistence.ArtifactBindingAdapter{Pool: pool}, func() time.Time { return now })
	receipt, err := service.InitiateUpload(ctx, app.InitiateUploadRequest{Declaration: app.UploadDeclaration{ArtifactID: "00000000-0000-0000-2411-000000000001", FirmwareVersion: "v1", Metadata: metadata}, GrantID: "upload-capability-1", Container: "artifacts", ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(receipt.Authorization, ".")
	if len(parts) != 2 {
		t.Fatalf("authorization does not contain signed grant")
	}
	claims, err := (localartifactstore.SignedGrant{Payload: parts[0], Signature: parts[1]}).Verify(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Key != "artifacts/v1/00000000-0000-0000-2411-000000000001.bin" || receipt.Object.Key() != claims.Key || receipt.Object.Digest() != digest {
		t.Fatalf("claims=%+v receipt=%+v", claims, receipt)
	}
	var key, state string
	if err := pool.QueryRow(ctx, "SELECT object_reference,state FROM firmware_artifacts WHERE id=$1::uuid", "00000000-0000-0000-2411-000000000001").Scan(&key, &state); err != nil || key != claims.Key || state != "UPLOADING" {
		t.Fatalf("declaration key=%q state=%q err=%v", key, state, err)
	}
	if _, err := store.PutSignedGrant(secret, localartifactstore.SignedGrant{Payload: parts[0], Signature: parts[1]}, now, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteUpload(ctx, "00000000-0000-0000-2411-000000000001", receipt.Object); err != nil {
		t.Fatal(err)
	}
	var versionID string
	if err := pool.QueryRow(ctx, "SELECT a.state,o.version_id FROM firmware_artifacts a JOIN artifact_object_versions o ON o.artifact_id=a.id WHERE a.id=$1::uuid", "00000000-0000-0000-2411-000000000001").Scan(&state, &versionID); err != nil || state != "UPLOADED" || versionID != receipt.Object.VersionID() {
		t.Fatalf("binding state=%q version=%q err=%v", state, versionID, err)
	}
}
