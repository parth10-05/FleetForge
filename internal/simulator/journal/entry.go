// Package journal defines a safe, hash-chained simulator evidence journal.
package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/parth10-05/fleetforge/internal/simulator/clock"
)

const SchemaVersionV1 = "fleetforge.simulator.journal/v1"

var ErrInvalidChain = errors.New("invalid simulator journal chain")

type Entry struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Sequence      uint64            `json:"sequence"`
	LogicalTime   clock.LogicalTime `json:"logical_time"`
	Kind          string            `json:"kind"`
	TargetID      string            `json:"target_id,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	RequestDigest string            `json:"request_digest,omitempty"`
	PreviousHash  string            `json:"previous_hash"`
	Hash          string            `json:"hash"`
}

func New(runID string, sequence uint64, logicalTime clock.LogicalTime, kind, targetID, correlationID, requestDigest, previousHash string) (Entry, error) {
	entry := Entry{SchemaVersion: SchemaVersionV1, RunID: runID, Sequence: sequence, LogicalTime: logicalTime, Kind: kind, TargetID: targetID, CorrelationID: correlationID, RequestDigest: requestDigest, PreviousHash: previousHash}
	if err := entry.validateFields(); err != nil {
		return Entry{}, err
	}
	hash, err := entry.computedHash()
	if err != nil {
		return Entry{}, err
	}
	entry.Hash = hash
	return entry, nil
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
	if e.SchemaVersion != SchemaVersionV1 || e.RunID == "" || e.Sequence == 0 || e.Kind == "" || containsUnsafe(e.TargetID) || containsUnsafe(e.CorrelationID) || containsUnsafe(e.RequestDigest) || containsUnsafe(e.PreviousHash) {
		return ErrInvalidChain
	}
	return nil
}

func containsUnsafe(raw string) bool {
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func Validate(entries []Entry) error {
	previous := ""
	for index, entry := range entries {
		if err := entry.validateFields(); err != nil || entry.Sequence != uint64(index+1) || entry.PreviousHash != previous {
			return ErrInvalidChain
		}
		expected, err := entry.computedHash()
		if err != nil || entry.Hash != expected {
			return ErrInvalidChain
		}
		previous = entry.Hash
	}
	return nil
}

func DecodeLines(raw []byte) ([]Entry, error) {
	var entries []Entry
	for _, line := range splitLines(raw) {
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("%w: JSON entry", ErrInvalidChain)
		}
		entries = append(entries, entry)
	}
	if err := Validate(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func splitLines(raw []byte) [][]byte {
	var lines [][]byte
	start := 0
	for index, value := range raw {
		if value == '\n' {
			if index > start {
				lines = append(lines, raw[start:index])
			}
			start = index + 1
		}
	}
	if start < len(raw) {
		lines = append(lines, raw[start:])
	}
	return lines
}
