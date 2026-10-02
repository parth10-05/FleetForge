package artifacts

import (
	"context"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestRunCleanupOnceResolvesAndDeletesOnlyClaimReceipt(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	digest, _ := value.ParseSHA256Digest("sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	identity, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "objects/a.bin", "version-1", 3, digest)
	claim := &cleanupClaimer{claim: &CleanupIntent{ArtifactID: "artifact-1", GrantID: "grant-1", LeaseToken: "token-1"}}
	receipts := &cleanupReceipts{identity: identity}
	deleter := &cleanupDeleter{}
	completer := &cleanupCompleter{}
	processed, err := RunCleanupOnce(context.Background(), claim, completer, receipts, deleter, "worker", "token-1", now, now.Add(time.Minute))
	if err != nil || !processed || receipts.grant != "grant-1" || deleter.identity.VersionID() != "version-1" || completer.claim.LeaseToken != "token-1" {
		t.Fatalf("processed=%v err=%v receipt=%q deleted=%+v completed=%+v", processed, err, receipts.grant, deleter.identity, completer.claim)
	}
}

type cleanupClaimer struct{ claim *CleanupIntent }

func (f *cleanupClaimer) ClaimArtifactCleanupIntent(context.Context, string, string, time.Time, time.Time) (*CleanupIntent, error) {
	return f.claim, nil
}

type cleanupCompleter struct{ claim CleanupIntent }

func (f *cleanupCompleter) CompleteArtifactCleanupIntent(_ context.Context, claim CleanupIntent, _ time.Time) error {
	f.claim = claim
	return nil
}

type cleanupReceipts struct {
	grant    string
	identity artifact.ObjectIdentity
}

func (f *cleanupReceipts) Receipt(grant string) (artifact.ObjectIdentity, error) {
	f.grant = grant
	return f.identity, nil
}

type cleanupDeleter struct{ identity artifact.ObjectIdentity }

func (f *cleanupDeleter) DeleteExact(identity artifact.ObjectIdentity) error {
	f.identity = identity
	return nil
}
