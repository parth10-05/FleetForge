package artifact

import (
	"errors"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestUploadMetadataBoundsSizeAndSeparatesDisplayNameFromObjectIdentity(t *testing.T) {
	digest, _ := value.ParseSHA256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	metadata, err := NewUploadMetadata("firmware-v2.bin", SyntheticFirmwareMediaType, 1024, digest)
	if err != nil || metadata.DisplayName() != "firmware-v2.bin" {
		t.Fatalf("metadata=%+v err=%v", metadata, err)
	}
	for _, input := range []struct {
		name, media string
		size        int64
	}{{"../escape", SyntheticFirmwareMediaType, 1}, {"firmware.bin", "text/plain", 1}, {"firmware.bin", SyntheticFirmwareMediaType, maxArtifactSize + 1}} {
		if _, err := NewUploadMetadata(input.name, input.media, input.size, digest); !errors.Is(err, ErrInvalidUploadMetadata) {
			t.Fatalf("input=%+v err=%v", input, err)
		}
	}
}
