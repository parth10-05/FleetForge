package artifact

import (
	"errors"
	"strings"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestObjectIdentityRequiresExactVersionAndBoundedTuple(t *testing.T) {
	digest, _ := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	identity, err := NewObjectIdentity(ObjectProviderLocalV1, "firmware", "artifacts/a.bin", "v0001", 1024, digest)
	if err != nil || identity.VersionID() != "v0001" {
		t.Fatalf("NewObjectIdentity() = %#v, %v", identity, err)
	}
	for _, input := range []struct {
		provider, container, key, version string
		size                              int64
	}{{"other", "firmware", "a", "v", 1}, {ObjectProviderLocalV1, "firmware", "../a", "v", 1}, {ObjectProviderLocalV1, "firmware", "a", "", 1}, {ObjectProviderLocalV1, "firmware", "a", "v", maxArtifactSize + 1}} {
		_, err := NewObjectIdentity(input.provider, input.container, input.key, input.version, input.size, digest)
		if !errors.Is(err, ErrInvalidObjectIdentity) {
			t.Fatalf("NewObjectIdentity(%+v) = %v", input, err)
		}
	}
}
