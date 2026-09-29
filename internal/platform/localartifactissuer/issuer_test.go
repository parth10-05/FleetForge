package localartifactissuer

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func TestIssuerSignsExactProviderAssignedObjectCapability(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	digest, err := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := artifact.NewUploadMetadata("firmware.bin", artifact.SyntheticFirmwareMediaType, 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	issuer := New(secret, bytes.NewReader(bytes.Repeat([]byte{0xab}, 16)))
	capability, err := issuer.IssueUploadCapability(context.Background(), app.UploadCapabilityRequest{GrantID: "grant-1", Container: "artifacts", Key: "artifacts/v1/a.bin", Metadata: metadata, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(capability.Authorization, ".")
	claims, err := (localartifactstore.SignedGrant{Payload: parts[0], Signature: parts[1]}).Verify(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Key != capability.Object.Key() || claims.VersionID != capability.Object.VersionID() || !strings.HasPrefix(claims.VersionID, "local-") {
		t.Fatalf("claims=%+v object=%+v", claims, capability.Object)
	}
}
