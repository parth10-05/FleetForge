// Package localartifactstore is the versioned in-process adapter used by P02
// contract tests. It is not a database and never exposes a bucket-wide API.
package localartifactstore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var (
	ErrVersionExists      = errors.New("artifact object version already exists")
	ErrVersionAbsent      = errors.New("artifact object version absent")
	ErrUploadGrantExpired = errors.New("artifact upload grant expired")
)

type Store struct {
	mu          sync.RWMutex
	versions    map[string][]byte
	nextVersion uint64
}

// UploadGrant is an opaque, one-object local direct-upload authorization.
// Its identity is intentionally not reconstructable by callers.
type UploadGrant struct {
	identity  artifact.ObjectIdentity
	pending   *pendingUpload
	expiresAt time.Time
}

type pendingUpload struct {
	provider, container, key string
	size                     int64
	digest                   value.SHA256Digest
}

func (store *Store) AuthorizeUpload(identity artifact.ObjectIdentity, now, expiresAt time.Time) (UploadGrant, error) {
	if now.IsZero() || !expiresAt.After(now) {
		return UploadGrant{}, ErrUploadGrantExpired
	}
	return UploadGrant{identity: identity, expiresAt: expiresAt.UTC()}, nil
}

func (store *Store) PutAuthorized(grant UploadGrant, now time.Time, source io.Reader) error {
	if now.IsZero() || !now.Before(grant.expiresAt) {
		return ErrUploadGrantExpired
	}
	return store.Put(grant.identity, source)
}

// AuthorizeNewUpload grants one exact new object key without preassigning a
// version ID. The version is assigned only by PutAuthorizedNew after content
// validation, matching a versioned object-store upload receipt.
func (store *Store) AuthorizeNewUpload(provider, container, key string, size int64, digest value.SHA256Digest, now, expiresAt time.Time) (UploadGrant, error) {
	if now.IsZero() || !expiresAt.After(now) {
		return UploadGrant{}, ErrUploadGrantExpired
	}
	if _, err := artifact.NewObjectIdentity(provider, container, key, "pending", size, digest); err != nil {
		return UploadGrant{}, err
	}
	return UploadGrant{pending: &pendingUpload{provider: provider, container: container, key: key, size: size, digest: digest}, expiresAt: expiresAt.UTC()}, nil
}

// PutAuthorizedNew writes exactly one authorized object and returns the
// provider-assigned immutable identity. The caller cannot select the version.
func (store *Store) PutAuthorizedNew(grant UploadGrant, now time.Time, source io.Reader) (artifact.ObjectIdentity, error) {
	if grant.pending == nil || now.IsZero() || !now.Before(grant.expiresAt) {
		return artifact.ObjectIdentity{}, ErrUploadGrantExpired
	}
	versionID := store.allocateVersionID()
	identity, err := artifact.NewObjectIdentity(grant.pending.provider, grant.pending.container, grant.pending.key, versionID, grant.pending.size, grant.pending.digest)
	if err != nil {
		return artifact.ObjectIdentity{}, err
	}
	if err := store.Put(identity, source); err != nil {
		return artifact.ObjectIdentity{}, err
	}
	return identity, nil
}

func New() *Store { return &Store{versions: make(map[string][]byte)} }

// Put writes exactly one immutable version. A reused key is allowed only with
// a distinct version ID; an existing tuple can never be overwritten.
func (store *Store) Put(identity artifact.ObjectIdentity, source io.Reader) error {
	if source == nil {
		return artifact.ErrObjectSizeMismatch
	}
	body, err := io.ReadAll(io.LimitReader(source, identity.Size()+1))
	if err != nil {
		return err
	}
	return store.PutBytes(identity, body)
}

// PutBytes copies a bounded object and verifies it before publication, so no
// partial version becomes observable.
func (store *Store) PutBytes(identity artifact.ObjectIdentity, body []byte) error {
	if err := artifact.VerifyContent(identity, bytes.NewReader(body)); err != nil {
		return err
	}
	key := identityKey(identity)
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.versions[key]; exists {
		return ErrVersionExists
	}
	store.versions[key] = append([]byte(nil), body...)
	return nil
}

func (store *Store) Open(identity artifact.ObjectIdentity) (io.ReadCloser, error) {
	store.mu.RLock()
	body, exists := store.versions[identityKey(identity)]
	store.mu.RUnlock()
	if !exists {
		return nil, ErrVersionAbsent
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

// Verify reopens exactly the bound object version and streams it through the
// domain verifier. A replacement under the same key has a new version ID and
// cannot satisfy this identity.
func (store *Store) Verify(identity artifact.ObjectIdentity) error {
	reader, err := store.Open(identity)
	if err != nil {
		return err
	}
	defer reader.Close()
	return artifact.VerifyContent(identity, reader)
}

// DeleteExact removes only the supplied immutable provider/container/key/
// version tuple. Callers must obtain this identity from an already-committed
// workflow decision; the store intentionally exposes no list or broad-delete
// operation that could remove verified or referenced versions by age alone.
func (store *Store) DeleteExact(identity artifact.ObjectIdentity) error {
	key := identityKey(identity)
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.versions[key]; !exists {
		return ErrVersionAbsent
	}
	delete(store.versions, key)
	return nil
}

func identityKey(identity artifact.ObjectIdentity) string {
	return identity.Provider() + "\x00" + identity.Container() + "\x00" + identity.Key() + "\x00" + identity.VersionID()
}

func (store *Store) allocateVersionID() string {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.nextVersion++
	return fmt.Sprintf("local-%020d", store.nextVersion)
}
