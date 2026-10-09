package execution

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type exactReaderFunc func(context.Context, ObjectIdentity) (io.ReadCloser, error)

func (f exactReaderFunc) OpenExact(ctx context.Context, object ObjectIdentity) (io.ReadCloser, error) {
	return f(ctx, object)
}

func TestOpenVerifiedExactAcceptsOnlyMatchingStream(t *testing.T) {
	object := ObjectIdentity{Provider: "local", Container: "c", ObjectKey: "k", ObjectVersion: "v", Size: 3, SHA256: "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}
	stream, err := OpenVerifiedExact(context.Background(), exactReaderFunc(func(context.Context, ObjectIdentity) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("abc"))), nil
	}), object)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := io.ReadAll(stream); err != nil || string(got) != "abc" {
		t.Fatalf("read=%q err=%v", got, err)
	}
}
func TestOpenVerifiedExactRejectsMismatchedDigest(t *testing.T) {
	object := ObjectIdentity{Provider: "local", Container: "c", ObjectKey: "k", ObjectVersion: "v", Size: 3, SHA256: "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}
	stream, err := OpenVerifiedExact(context.Background(), exactReaderFunc(func(context.Context, ObjectIdentity) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("bad"))), nil
	}), object)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(stream)
	if !errors.Is(err, ErrExactArtifactMismatch) {
		t.Fatalf("err=%v", err)
	}
}
