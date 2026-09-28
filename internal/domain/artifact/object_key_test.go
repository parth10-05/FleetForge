package artifact

import (
	"errors"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestUploadObjectKeyUsesOnlyValidatedArtifactIdentity(t *testing.T) {
	id, _ := value.ParseArtifactID("00000000-0000-0000-0000-000000000001")
	got, err := UploadObjectKey(id)
	if err != nil || got != "artifacts/v1/00000000-0000-0000-0000-000000000001.bin" {
		t.Fatalf("key=%q err=%v", got, err)
	}
	if _, err := UploadObjectKey(value.ArtifactID{}); !errors.Is(err, ErrInvalidUploadObjectKey) {
		t.Fatalf("zero id error=%v", err)
	}
}
