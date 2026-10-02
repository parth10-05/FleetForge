package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
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

func TestVerifyContentStreamsMaximumArtifactWithoutLargeReadBuffer(t *testing.T) {
	chunk := make([]byte, 64<<10)
	for index := range chunk {
		chunk[index] = 0xa5
	}
	hash := sha256.New()
	for remaining := int64(maxArtifactSize); remaining > 0; {
		part := partialChunk(chunk, remaining)
		_, _ = hash.Write(part)
		remaining -= int64(len(part))
	}
	digest, err := value.ParseSHA256Digest("sha256:" + hex.EncodeToString(hash.Sum(nil)))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := NewObjectIdentity(ObjectProviderLocalV1, "artifacts", "maximum.bin", "version-maximum", maxArtifactSize, digest)
	if err != nil {
		t.Fatal(err)
	}
	source := &repeatingReader{remaining: maxArtifactSize, byte: 0xa5}
	if err := VerifyContent(identity, source); err != nil {
		t.Fatal(err)
	}
	if source.remaining != 0 || source.maxRead > 64<<10 {
		t.Fatalf("remaining=%d largest read=%d", source.remaining, source.maxRead)
	}
}

type repeatingReader struct {
	remaining int64
	byte      byte
	maxRead   int
}

func (reader *repeatingReader) Read(destination []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	if len(destination) > reader.maxRead {
		reader.maxRead = len(destination)
	}
	count := int64(len(destination))
	if count > reader.remaining {
		count = reader.remaining
	}
	for index := range destination[:count] {
		destination[index] = reader.byte
	}
	reader.remaining -= count
	return int(count), nil
}

func partialChunk(chunk []byte, remaining int64) []byte {
	if remaining < int64(len(chunk)) {
		return chunk[:remaining]
	}
	return chunk
}
