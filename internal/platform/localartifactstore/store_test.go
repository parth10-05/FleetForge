package localartifactstore

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestStorePreservesExactVersionAndRejectsOverwrite(t *testing.T) {
	digest, _ := value.ParseSHA256Digest("sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
	first, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "same-key", "v1", 11, digest)
	second, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "same-key", "v2", 11, digest)
	store := New()
	if err := store.PutBytes(first, []byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if err := store.PutBytes(first, []byte("hello world")); !errors.Is(err, ErrVersionExists) {
		t.Fatalf("overwrite = %v", err)
	}
	if err := store.PutBytes(second, []byte("hello world")); err != nil {
		t.Fatal(err)
	}
	reader, err := store.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, _ := io.ReadAll(reader)
	if string(body) != "hello world" {
		t.Fatalf("body=%q", body)
	}
	if err := store.Verify(first); err != nil {
		t.Fatal(err)
	}
	missing, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "same-key", "v3", 11, digest)
	if err := store.Verify(missing); !errors.Is(err, ErrVersionAbsent) {
		t.Fatalf("different version verification=%v", err)
	}
}

func TestUploadGrantExpiresBeforeObjectWrite(t *testing.T) {
	digest, _ := value.ParseSHA256Digest("sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
	identity, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "ticketed", "v1", 11, digest)
	store := New()
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	grant, err := store.AuthorizeUpload(identity, now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutAuthorized(grant, now, strings.NewReader("hello world")); err != nil {
		t.Fatal(err)
	}
	expired, _ := store.AuthorizeUpload(identity, now, now.Add(time.Second))
	if err := store.PutAuthorized(expired, now.Add(time.Second), strings.NewReader("hello world")); !errors.Is(err, ErrUploadGrantExpired) {
		t.Fatalf("expired grant error=%v", err)
	}
}

func TestNewUploadGrantAssignsVersionOnlyAfterExactContentAccepted(t *testing.T) {
	digest, _ := value.ParseSHA256Digest("sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
	store := New()
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	grant, err := store.AuthorizeNewUpload(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/new.bin", 11, digest, now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := store.PutAuthorizedNew(grant, now, strings.NewReader("hello world"))
	if err != nil || identity.VersionID() == "" || identity.VersionID() == "pending" {
		t.Fatalf("identity=%+v err=%v", identity, err)
	}
	if err := store.Verify(identity); err != nil {
		t.Fatal(err)
	}
	badGrant, err := store.AuthorizeNewUpload(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/bad.bin", 11, digest, now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutAuthorizedNew(badGrant, now, strings.NewReader("wrong bytes")); !errors.Is(err, artifact.ErrObjectDigestMismatch) {
		t.Fatalf("bad content error=%v", err)
	}
}

func TestDeleteExactCannotRemoveSiblingVersion(t *testing.T) {
	digest, _ := value.ParseSHA256Digest("sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
	first, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "objects/shared.bin", "version-1", 11, digest)
	second, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "objects/shared.bin", "version-2", 11, digest)
	store := New()
	if err := store.PutBytes(first, []byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if err := store.PutBytes(second, []byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteExact(first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(first); !errors.Is(err, ErrVersionAbsent) {
		t.Fatalf("deleted version open error=%v", err)
	}
	if err := store.Verify(second); err != nil {
		t.Fatalf("sibling version verification=%v", err)
	}
}
