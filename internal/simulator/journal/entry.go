// Package journal defines the safe, hash-chained simulator evidence journal.
package journal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/clock"
)

const SchemaVersionV2 = "fleetforge.simulator.journal/v2"
const SchemaVersionV3 = "fleetforge.simulator.journal/v3"
const SchemaVersionCurrent = SchemaVersionV3
const SchemaVersionV1 = SchemaVersionV2 // retained name prevents silent v1 callers.

var (
	ErrInvalidChain  = errors.New("invalid simulator journal chain")
	ErrLimitExceeded = errors.New("simulator journal entry limit exceeded")
)

type Kind string

const (
	RunCreated          Kind = "run_created"
	RequestReceived     Kind = "request_received"
	RequestRejected     Kind = "request_rejected"
	UpdateAccepted      Kind = "update_accepted"
	TaskTransition      Kind = "task_transition"
	RebootTransition    Kind = "reboot_transition"
	InventoryTransition Kind = "inventory_transition"
	HealthTransition    Kind = "health_transition"
	FaultConsumed       Kind = "fault_consumed"
	SubscriptionCreated Kind = "subscription_created"
	EventQueued         Kind = "event_queued"
	EventDelivered      Kind = "event_delivered"
	SnapshotWritten     Kind = "snapshot_written"
	RunInvalidated      Kind = "run_invalidated"
	Diagnostic          Kind = "diagnostic"
)

func (k Kind) valid() bool {
	switch k {
	case RunCreated, RequestReceived, RequestRejected, UpdateAccepted, TaskTransition, RebootTransition, InventoryTransition, HealthTransition, FaultConsumed, SubscriptionCreated, EventQueued, EventDelivered, SnapshotWritten, RunInvalidated, Diagnostic:
		return true
	}
	return false
}
func (k Kind) syncRequired() bool { return k == RequestReceived || k == UpdateAccepted }

// Payload contains only bounded metadata and fingerprints. No credentials,
// URLs, or artifact bytes have a representation in this record format.
type Payload struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	Detail        string `json:"detail,omitempty"`
}
type Entry struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Sequence      uint64            `json:"sequence"`
	LogicalTime   clock.LogicalTime `json:"logical_time"`
	RecordedAt    time.Time         `json:"recorded_at,omitempty"`
	Kind          Kind              `json:"kind"`
	TargetID      string            `json:"target_id,omitempty"`
	Payload       Payload           `json:"payload,omitempty"`
	PreviousHash  string            `json:"previous_hash"`
	Hash          string            `json:"hash"`
}
type Record struct {
	LogicalTime clock.LogicalTime
	RecordedAt  time.Time
	Kind        Kind
	TargetID    string
	Payload     Payload
}

func New(runID string, sequence uint64, logicalTime clock.LogicalTime, kind Kind, targetID, correlationID, requestDigest, previousHash string) (Entry, error) {
	return newEntry(runID, sequence, Record{LogicalTime: logicalTime, RecordedAt: time.Now().UTC(), Kind: kind, TargetID: targetID, Payload: Payload{CorrelationID: correlationID, Fingerprint: requestDigest}}, previousHash)
}
func newEntry(runID string, sequence uint64, record Record, previous string) (Entry, error) {
	recordedAt := record.RecordedAt.UTC()
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}
	e := Entry{SchemaVersion: SchemaVersionCurrent, RunID: runID, Sequence: sequence, LogicalTime: record.LogicalTime, RecordedAt: recordedAt, Kind: record.Kind, TargetID: record.TargetID, Payload: record.Payload, PreviousHash: previous}
	if err := e.validateFields(); err != nil {
		return Entry{}, err
	}
	hash, err := e.computedHash()
	if err != nil {
		return Entry{}, err
	}
	e.Hash = hash
	return e, nil
}
func (e Entry) computedHash() (string, error) {
	e.Hash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func (e Entry) validateFields() error {
	if (e.SchemaVersion != SchemaVersionV2 && e.SchemaVersion != SchemaVersionV3) || !e.Kind.valid() || e.RunID == "" || e.Sequence == 0 || unsafe(e.RunID, 128) || unsafe(e.TargetID, 128) || unsafe(e.Payload.CorrelationID, 128) || unsafe(e.Payload.Fingerprint, 128) || unsafe(e.Payload.Detail, 256) || unsafe(e.PreviousHash, 64) || strings.Contains(e.Payload.Fingerprint, "://") {
		return ErrInvalidChain
	}
	if e.SchemaVersion == SchemaVersionV3 && e.RecordedAt.IsZero() {
		return ErrInvalidChain
	}
	if e.Hash != "" && len(e.Hash) != 64 {
		return ErrInvalidChain
	}
	return nil
}
func unsafe(s string, max int) bool {
	if len(s) > max {
		return true
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	lower := strings.ToLower(s)
	return strings.Contains(lower, "password") || strings.Contains(lower, "authorization") || strings.Contains(lower, "secret")
}
func Validate(entries []Entry) error {
	previous := ""
	for i, e := range entries {
		if e.validateFields() != nil || e.Sequence != uint64(i+1) || e.PreviousHash != previous {
			return ErrInvalidChain
		}
		expected, err := e.computedHash()
		if err != nil || e.Hash != expected {
			return ErrInvalidChain
		}
		previous = e.Hash
	}
	return nil
}
func DecodeLines(raw []byte) ([]Entry, error) {
	lines := bytes.Split(raw, []byte{'\n'})
	entries := make([]Entry, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		var e Entry
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("%w: JSON entry", ErrInvalidChain)
		}
		var extra any
		if err := dec.Decode(&extra); err == nil {
			return nil, fmt.Errorf("%w: extra JSON", ErrInvalidChain)
		}
		entries = append(entries, e)
	}
	if err := Validate(entries); err != nil {
		return nil, err
	}
	return entries, nil
}
