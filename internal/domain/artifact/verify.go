package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var (
	ErrObjectSizeMismatch   = errors.New("artifact object size mismatch")
	ErrObjectDigestMismatch = errors.New("artifact object digest mismatch")
)

// VerifyContent streams one exact immutable object version through SHA-256.
// It keeps no firmware bytes, reads at most identity.Size()+1 bytes, and
// returns only safe verification metadata.
func VerifyContent(identity ObjectIdentity, source io.Reader) error {
	if source == nil || identity.VersionID() == "" || identity.Size() <= 0 {
		return ErrObjectSizeMismatch
	}
	hash := sha256.New()
	read, err := io.Copy(hash, io.LimitReader(source, identity.Size()+1))
	if err != nil || read != identity.Size() {
		return ErrObjectSizeMismatch
	}
	digest, err := value.ParseSHA256Digest("sha256:" + hex.EncodeToString(hash.Sum(nil)))
	if err != nil || digest != identity.Digest() {
		return ErrObjectDigestMismatch
	}
	return nil
}
