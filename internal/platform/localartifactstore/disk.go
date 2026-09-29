package localartifactstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	root     string
	mu       sync.Mutex
	receipts map[string]artifact.ObjectIdentity
}

const receiptsFilename = ".fleetforge-upload-receipts.json"

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
	if source == nil {
		return artifact.ObjectIdentity{}, artifact.ErrObjectSizeMismatch
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if received, exists := store.receipts[claims.GrantID]; exists {
		if identityKey(received) != identityKey(identity) {
			return artifact.ObjectIdentity{}, ErrInvalidGrant
		}
		if err := artifact.VerifyContent(received, io.LimitReader(source, received.Size()+1)); err != nil {
			return artifact.ObjectIdentity{}, err
		}
		return received, nil
	}
	if err := store.putLocked(identity, source); err != nil {
		return artifact.ObjectIdentity{}, err
	}
	store.receipts[claims.GrantID] = identity
	if err := store.persistReceiptsLocked(); err != nil {
		delete(store.receipts, claims.GrantID)
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
	store := &DiskStore{root: clean, receipts: make(map[string]artifact.ObjectIdentity)}
	if err := store.loadReceipts(); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *DiskStore) Put(identity artifact.ObjectIdentity, source io.Reader) error {
	if source == nil {
		return artifact.ErrObjectSizeMismatch
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.putLocked(identity, source)
}

func (store *DiskStore) putLocked(identity artifact.ObjectIdentity, source io.Reader) error {
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
	if err := temp.Sync(); err != nil {
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

type receiptManifest struct {
	Version  string                   `json:"version"`
	Receipts map[string]receiptRecord `json:"receipts"`
}

type receiptRecord struct {
	Provider  string `json:"provider"`
	Container string `json:"container"`
	Key       string `json:"key"`
	VersionID string `json:"version_id"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

func (store *DiskStore) loadReceipts() error {
	body, err := os.ReadFile(store.receiptsPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var manifest receiptManifest
	if err := json.Unmarshal(body, &manifest); err != nil || manifest.Version != "fleetforge.local-upload-receipts/v1" || manifest.Receipts == nil {
		return errors.New("invalid local artifact upload receipt manifest")
	}
	for grantID, record := range manifest.Receipts {
		if grantID == "" {
			return errors.New("invalid local artifact upload receipt manifest")
		}
		digest, err := value.ParseSHA256Digest(record.Digest)
		if err != nil {
			return errors.New("invalid local artifact upload receipt manifest")
		}
		identity, err := artifact.NewObjectIdentity(record.Provider, record.Container, record.Key, record.VersionID, record.Size, digest)
		if err != nil {
			return errors.New("invalid local artifact upload receipt manifest")
		}
		if _, err := os.Lstat(store.path(identity)); err != nil {
			return errors.New("local artifact receipt references missing object")
		}
		store.receipts[grantID] = identity
	}
	return nil
}

func (store *DiskStore) persistReceiptsLocked() error {
	manifest := receiptManifest{Version: "fleetforge.local-upload-receipts/v1", Receipts: make(map[string]receiptRecord, len(store.receipts))}
	for grantID, identity := range store.receipts {
		manifest.Receipts[grantID] = receiptRecord{Provider: identity.Provider(), Container: identity.Container(), Key: identity.Key(), VersionID: identity.VersionID(), Size: identity.Size(), Digest: identity.Digest().String()}
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(store.root, ".receipts-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, store.receiptsPath())
}

func (store *DiskStore) receiptsPath() string { return filepath.Join(store.root, receiptsFilename) }

func (store *DiskStore) Open(identity artifact.ObjectIdentity) (io.ReadCloser, error) {
	file, err := os.Open(store.path(identity))
	if os.IsNotExist(err) {
		return nil, ErrVersionAbsent
	}
	return file, err
}

// Verify reopens only the exact immutable version and streams it through the
// domain verifier. It exposes neither a directory nor a mutable object key.
func (store *DiskStore) Verify(identity artifact.ObjectIdentity) error {
	reader, err := store.Open(identity)
	if err != nil {
		return err
	}
	defer reader.Close()
	return artifact.VerifyContent(identity, reader)
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
