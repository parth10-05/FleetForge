package localartifactstore

import (
	"strings"
	"testing"
	"time"
)

func TestSignedGrantBindsClaimsAndExpires(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	claims := UploadGrantClaims{Version: "fleetforge.upload-grant/v1", GrantID: "grant-1", Method: "PUT", Provider: "local-versioned-v1", Container: "artifacts", Key: "objects/a.bin", VersionID: "version-1", Size: 11, Digest: "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", MediaType: "application/octet-stream", ExpiresAt: now.Add(time.Minute)}
	grant, err := IssueSignedGrant(secret, claims)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := grant.Verify(secret, now); err != nil || got.GrantID != claims.GrantID {
		t.Fatalf("verify=%+v err=%v", got, err)
	}
	if _, err := grant.Verify(secret, now.Add(time.Minute)); err == nil {
		t.Fatal("expired grant accepted")
	}
	grant.Signature = "tampered"
	if _, err := grant.Verify(secret, now); err == nil {
		t.Fatal("tampered grant accepted")
	}
}

func TestDiskStoreAcceptsOnlySignedExactGrant(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	claims := UploadGrantClaims{Version: "fleetforge.upload-grant/v1", GrantID: "grant-2", Method: "PUT", Provider: "local-versioned-v1", Container: "artifacts", Key: "objects/b.bin", VersionID: "version-1", Size: 11, Digest: "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", MediaType: "application/octet-stream", ExpiresAt: now.Add(time.Minute)}
	grant, _ := IssueSignedGrant(secret, claims)
	store, err := OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutSignedGrant(secret, grant, now, strings.NewReader("hello world")); err != nil {
		t.Fatal(err)
	}
	if received, err := store.PutSignedGrant(secret, grant, now, strings.NewReader("hello world")); err != nil || received.VersionID() != claims.VersionID {
		t.Fatalf("same grant retry receipt=%+v err=%v", received, err)
	}
	if _, err := store.PutSignedGrant(secret, grant, now, strings.NewReader("wrong bytes")); err == nil {
		t.Fatal("grant replay accepted different bytes")
	}
}

func TestDiskStoreRecoversDurableGrantReceipt(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	claims := UploadGrantClaims{Version: "fleetforge.upload-grant/v1", GrantID: "grant-recovery", Method: "PUT", Provider: "local-versioned-v1", Container: "artifacts", Key: "objects/recovery.bin", VersionID: "version-1", Size: 11, Digest: "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", MediaType: "application/octet-stream", ExpiresAt: now.Add(time.Minute)}
	grant, err := IssueSignedGrant(secret, claims)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := OpenDisk(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutSignedGrant(secret, grant, now, strings.NewReader("hello world")); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDisk(root)
	if err != nil {
		t.Fatalf("reopen=%v", err)
	}
	identity, err := reopened.PutSignedGrant(secret, grant, now, strings.NewReader("hello world"))
	if err != nil || identity.VersionID() != claims.VersionID {
		t.Fatalf("recovered receipt=%+v err=%v", identity, err)
	}
}
