package artifact

import (
	"errors"
	"strings"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const (
	ObjectProviderLocalV1                     = "local-versioned-v1"
	VerificationPolicySyntheticSHA256V1       = "synthetic-sha256-v1"
	maxObjectIdentityPartLength               = 256
	maxArtifactSize                     int64 = 16 << 20
)

var ErrInvalidObjectIdentity = errors.New("invalid immutable object identity")

// ObjectIdentity is the immutable tuple every later HEAD, GET, or delete must
// use. Digest alone is intentionally insufficient evidence of object identity.
type ObjectIdentity struct {
	provider  string
	container string
	key       string
	versionID string
	size      int64
	digest    value.SHA256Digest
}

func NewObjectIdentity(provider, container, key, versionID string, size int64, digest value.SHA256Digest) (ObjectIdentity, error) {
	if provider != ObjectProviderLocalV1 || !validObjectPart(container) || !validObjectPart(key) || !validObjectPart(versionID) || size <= 0 || size > maxArtifactSize || digest.IsZero() {
		return ObjectIdentity{}, ErrInvalidObjectIdentity
	}
	return ObjectIdentity{provider: provider, container: container, key: key, versionID: versionID, size: size, digest: digest}, nil
}

func (identity ObjectIdentity) Provider() string           { return identity.provider }
func (identity ObjectIdentity) Container() string          { return identity.container }
func (identity ObjectIdentity) Key() string                { return identity.key }
func (identity ObjectIdentity) VersionID() string          { return identity.versionID }
func (identity ObjectIdentity) Size() int64                { return identity.size }
func (identity ObjectIdentity) Digest() value.SHA256Digest { return identity.digest }

func validObjectPart(raw string) bool {
	return len(raw) > 0 && len(raw) <= maxObjectIdentityPartLength && !strings.ContainsAny(raw, "\\\\\x00\r\n") && !strings.Contains(raw, "..")
}
