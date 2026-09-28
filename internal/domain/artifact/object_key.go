package artifact

import (
	"errors"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var ErrInvalidUploadObjectKey = errors.New("invalid generated artifact object key")

// UploadObjectKey derives the only writable v1 object name from an artifact
// identity. Client display names and filenames cannot influence this path.
func UploadObjectKey(id value.ArtifactID) (string, error) {
	if id.IsZero() {
		return "", ErrInvalidUploadObjectKey
	}
	return "artifacts/v1/" + id.String() + ".bin", nil
}
