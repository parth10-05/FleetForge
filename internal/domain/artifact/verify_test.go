package artifact

import (
	"errors"
	"strings"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestVerifyContentStreamsExactImmutableObject(t *testing.T) {
	digest, _ := value.ParseSHA256Digest("sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
	identity, err := NewObjectIdentity(ObjectProviderLocalV1, "artifacts", "fixture.bin", "version-1", 11, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyContent(identity, strings.NewReader("hello world")); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"hello", "hello world!"} {
		if !errors.Is(VerifyContent(identity, strings.NewReader(body)), ErrObjectSizeMismatch) {
			t.Fatalf("body %q did not fail size check", body)
		}
	}
	if !errors.Is(VerifyContent(identity, strings.NewReader("HELLO WORLD")), ErrObjectDigestMismatch) {
		t.Fatal("digest mismatch accepted")
	}
}
