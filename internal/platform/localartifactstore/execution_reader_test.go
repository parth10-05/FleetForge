package localartifactstore

import (
	"context"
	"io"
	"testing"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestExecutionReaderOpensOnlyPersistedObjectVersion(t *testing.T) {
	digest, err := value.ParseSHA256Digest("sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "firmware", "verified/a.bin", "v1", 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	store := New()
	if err := store.PutBytes(identity, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	reader, err := (ExecutionReader{Store: store}).OpenExact(context.Background(), execution.ObjectIdentity{Provider: "local-versioned-v1", Container: "firmware", ObjectKey: "verified/a.bin", ObjectVersion: "v1", Size: 3, SHA256: digest.String()})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != "abc" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestExecutionReaderRejectsMalformedPersistedIdentity(t *testing.T) {
	reader := ExecutionReader{Store: New()}
	if _, err := reader.OpenExact(context.Background(), execution.ObjectIdentity{Provider: "local-versioned-v1", Container: "firmware", ObjectKey: "../wrong", ObjectVersion: "v1", Size: 1, SHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}); err == nil {
		t.Fatal("malformed identity was accepted")
	}
}
