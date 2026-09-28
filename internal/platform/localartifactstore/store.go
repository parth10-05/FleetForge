// Package localartifactstore is the versioned in-process adapter used by P02
// contract tests. It is not a database and never exposes a bucket-wide API.
package localartifactstore

import (
	"bytes"
	"errors"
	"io"
	"sync"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

var (
	ErrVersionExists = errors.New("artifact object version already exists")
	ErrVersionAbsent = errors.New("artifact object version absent")
)

type Store struct {
	mu       sync.RWMutex
	versions map[string][]byte
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

func identityKey(identity artifact.ObjectIdentity) string {
	return identity.Provider() + "\x00" + identity.Container() + "\x00" + identity.Key() + "\x00" + identity.VersionID()
}
