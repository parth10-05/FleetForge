package artifact

import (
	"bytes"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func FuzzVerifyContentRejectsUnexpectedBytesWithoutPanic(f *testing.F) {
	digest, _ := value.ParseSHA256Digest("sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
	identity, _ := NewObjectIdentity(ObjectProviderLocalV1, "artifacts", "fuzz.bin", "v1", 11, digest)
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("hello world!"))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1024 {
			return
		}
		_ = VerifyContent(identity, bytes.NewReader(body))
	})
}
