package localartifactstore

import (
	"errors"
	"io"
	"testing"

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
