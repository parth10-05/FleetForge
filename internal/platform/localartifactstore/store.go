// Package localartifactstore is the versioned in-process adapter used by P02
// contract tests. It is not a database and never exposes a bucket-wide API.
package localartifactstore

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

var (
	ErrVersionExists      = errors.New("artifact object version already exists")
	ErrVersionAbsent      = errors.New("artifact object version absent")
	ErrUploadGrantExpired = errors.New("artifact upload grant expired")
)

type Store struct {
	mu       sync.RWMutex
	versions map[string][]byte
}

// UploadGrant is an opaque, one-object local direct-upload authorization.
// Its identity is intentionally not reconstructable by callers.
type UploadGrant struct {
	identity  artifact.ObjectIdentity
	expiresAt time.Time
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

func identityKey(identity artifact.ObjectIdentity) string {
	return identity.Provider() + "\x00" + identity.Container() + "\x00" + identity.Key() + "\x00" + identity.VersionID()
}
