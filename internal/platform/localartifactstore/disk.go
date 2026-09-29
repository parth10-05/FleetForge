package localartifactstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// DiskStore is the local P02 data-plane persistence primitive. It publishes an
// object only after the complete bounded stream verifies against its immutable
// identity; callers never supply a filesystem path.
type DiskStore struct {
	root string
	mu   sync.Mutex
}

// PutSignedGrant verifies the capability before deriving the only object
// identity accepted for publication. The request body cannot choose a path,
// version, digest, or size independently of the signed grant.
func (store *DiskStore) PutSignedGrant(secret []byte, grant SignedGrant, now time.Time, source io.Reader) (artifact.ObjectIdentity, error) {
	claims, err := grant.Verify(secret, now)
	if err != nil {
		return artifact.ObjectIdentity{}, err
	}
	digest, err := value.ParseSHA256Digest(claims.Digest)
	if err != nil {
		return artifact.ObjectIdentity{}, ErrInvalidGrant
	}
	identity, err := artifact.NewObjectIdentity(claims.Provider, claims.Container, claims.Key, claims.VersionID, claims.Size, digest)
	if err != nil {
		return artifact.ObjectIdentity{}, ErrInvalidGrant
	}
	if err := store.Put(identity, source); err != nil {
		return artifact.ObjectIdentity{}, err
	}
	return identity, nil
}

func OpenDisk(root string) (*DiskStore, error) {
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("local artifact root must be absolute")
	}
	clean := filepath.Clean(root)
	info, err := os.Lstat(clean)
	if err == nil && !info.IsDir() {
		return nil, errors.New("local artifact root is not a directory")
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return nil, err
	}
	return &DiskStore{root: clean}, nil
}

func (store *DiskStore) Put(identity artifact.ObjectIdentity, source io.Reader) error {
	if source == nil {
		return artifact.ErrObjectSizeMismatch
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	final := store.path(identity)
	if _, err := os.Lstat(final); err == nil {
		return ErrVersionExists
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(store.root, ".upload-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := io.Copy(temp, io.LimitReader(source, identity.Size()+1)); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	read, err := os.Open(tempName)
	if err != nil {
		return err
	}
	err = artifact.VerifyContent(identity, read)
	closeErr := read.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tempName, final); err != nil {
		return err
	}
	return nil
}

func (store *DiskStore) Open(identity artifact.ObjectIdentity) (io.ReadCloser, error) {
	file, err := os.Open(store.path(identity))
	if os.IsNotExist(err) {
		return nil, ErrVersionAbsent
	}
	return file, err
}

func (store *DiskStore) DeleteExact(identity artifact.ObjectIdentity) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	err := os.Remove(store.path(identity))
	if os.IsNotExist(err) {
		return ErrVersionAbsent
	}
	return err
}

func (store *DiskStore) path(identity artifact.ObjectIdentity) string {
	sum := sha256.Sum256([]byte(identityKey(identity)))
	return filepath.Join(store.root, hex.EncodeToString(sum[:])+".object")
}
