package journal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Writer validates a retained journal once, then owns sequence/hash allocation
// and serialized append. It is the only production append path.
type Writer struct {
	file            *os.File
	runID, previous string
	next            uint64
	limit           uint64
	mu              sync.Mutex
	closed          bool
	syncs           uint64
}

func OpenWriter(path, runID string) (*Writer, error) {
	return OpenWriterLimited(path, runID, 1_000_000)
}

// OpenWriterLimited retains the validated existing chain and refuses every
// append beyond the scenario's explicit evidence bound.
func OpenWriterLimited(path, runID string, limit uint64) (*Writer, error) {
	if runID == "" {
		return nil, ErrInvalidChain
	}
	if limit == 0 {
		return nil, ErrLimitExceeded
	}
	entries, err := Load(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(entries) > 0 && entries[0].RunID != runID {
		return nil, ErrInvalidChain
	}
	if uint64(len(entries)) > limit {
		return nil, ErrLimitExceeded
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	w := &Writer{file: f, runID: runID, next: 1, limit: limit}
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		w.next = last.Sequence + 1
		w.previous = last.Hash
	}
	return w, nil
}
func (w *Writer) Append(record Record) (Entry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return Entry{}, os.ErrClosed
	}
	if w.next > w.limit {
		return Entry{}, ErrLimitExceeded
	}
	entry, err := newEntry(w.runID, w.next, record, w.previous)
	if err != nil {
		return Entry{}, err
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return Entry{}, err
	}
	if _, err = w.file.Write(append(raw, '\n')); err != nil {
		w.poison()
		return Entry{}, err
	}
	if record.Kind.syncRequired() {
		if err = w.file.Sync(); err != nil {
			w.poison()
			return Entry{}, err
		}
		w.syncs++
	}
	w.next++
	w.previous = entry.Hash
	return entry, nil
}
func (w *Writer) poison() { w.closed = true; _ = w.file.Close() }
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.file.Close()
}
func (w *Writer) SyncCount() uint64 { w.mu.Lock(); defer w.mu.Unlock(); return w.syncs }

// Append is a compatibility helper for preparation tests. New application code
// must retain one Writer for the entire run.
func Append(path string, entry Entry) error {
	w, err := OpenWriter(path, entry.RunID)
	if err != nil {
		return err
	}
	defer w.Close()
	actual, err := w.Append(Record{LogicalTime: entry.LogicalTime, Kind: entry.Kind, TargetID: entry.TargetID, Payload: entry.Payload})
	if err != nil {
		return err
	}
	if actual.Hash != entry.Hash {
		return ErrInvalidChain
	}
	return nil
}
func Load(path string) ([]Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeLines(raw)
}

// ReplayAccepted projects only durable acceptance truth from an already
// validated chain. Each record remains distinct even when correlation inputs
// match; callers must not use this projection for deduplication.
func ReplayAccepted(entries []Entry) ([]Entry, error) {
	if err := Validate(entries); err != nil {
		return nil, err
	}
	accepted := make([]Entry, 0)
	for _, entry := range entries {
		if entry.Kind == UpdateAccepted {
			accepted = append(accepted, entry)
		}
	}
	return accepted, nil
}

type Snapshot struct {
	SchemaVersion string          `json:"schema_version"`
	Sequence      uint64          `json:"sequence"`
	Hash          string          `json:"hash"`
	LogicalTime   int64           `json:"logical_time"`
	StateDigest   string          `json:"state_digest"`
	State         json.RawMessage `json:"state"`
}

func WriteSnapshot(path string, s Snapshot) error {
	if s.SchemaVersion == "" {
		s.SchemaVersion = SchemaVersionV2
	}
	if len(s.State) > 0 && s.StateDigest == "" {
		digest := sha256.Sum256(s.State)
		s.StateDigest = hex.EncodeToString(digest[:])
	}
	if s.SchemaVersion != SchemaVersionV2 || (s.Sequence == 0 && s.Hash != "") || (s.Sequence > 0 && len(s.Hash) != 64) || len(s.StateDigest) > 64 || (len(s.StateDigest) > 0 && len(s.StateDigest) != 64) {
		return fmt.Errorf("invalid simulator snapshot")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = tmp.Write(raw); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func ReadSnapshot(path string) (Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("invalid simulator snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Snapshot{}, fmt.Errorf("invalid simulator snapshot")
	}
	hasState := len(snapshot.State) > 0 && !bytes.Equal(snapshot.State, []byte("null"))
	if snapshot.SchemaVersion != SchemaVersionV2 || (snapshot.Sequence == 0 && snapshot.Hash != "") || (snapshot.Sequence > 0 && len(snapshot.Hash) != 64) || (hasState && len(snapshot.StateDigest) != 64) {
		return Snapshot{}, fmt.Errorf("invalid simulator snapshot")
	}
	if len(snapshot.StateDigest) > 0 {
		digest := sha256.Sum256(snapshot.State)
		if snapshot.StateDigest != hex.EncodeToString(digest[:]) {
			return Snapshot{}, fmt.Errorf("invalid simulator snapshot")
		}
	}
	return snapshot, nil
}
