package localartifactstore

import (
	"context"
	"fmt"
	"io"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// ExecutionReader exposes only the exact immutable object tuple persisted in
// a firmware attempt. It has no latest-version or prefix lookup fallback.
type ExecutionReader struct {
	Store interface {
		Open(artifact.ObjectIdentity) (io.ReadCloser, error)
	}
}

func (reader ExecutionReader) OpenExact(_ context.Context, object execution.ObjectIdentity) (io.ReadCloser, error) {
	if reader.Store == nil {
		return nil, fmt.Errorf("invalid execution artifact reader")
	}
	digest, err := value.ParseSHA256Digest(object.SHA256)
	if err != nil {
		return nil, fmt.Errorf("parse persisted artifact digest: %w", err)
	}
	identity, err := artifact.NewObjectIdentity(object.Provider, object.Container, object.ObjectKey, object.ObjectVersion, object.Size, digest)
	if err != nil {
		return nil, fmt.Errorf("validate persisted artifact identity: %w", err)
	}
	return reader.Store.Open(identity)
}

var _ execution.ExactArtifactReader = ExecutionReader{}
