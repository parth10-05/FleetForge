// Package simulatorjournal adapts the retained, hash-chained simulator
// journal into independent execution-reconciliation evidence.
package simulatorjournal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
)

// Reader is configured with one retained simulator run. It does not share the
// Redfish transport path used to submit firmware, so its evidence is an
// independent oracle for ambiguity reconciliation.
type Reader struct{ Path string }

func (reader Reader) ReadAcceptance(ctx context.Context, runID, targetID string, from, until time.Time) (execution.AcceptanceJournal, error) {
	if err := ctx.Err(); err != nil {
		return execution.AcceptanceJournal{}, err
	}
	if reader.Path == "" || runID == "" || targetID == "" || from.IsZero() || until.IsZero() || until.Before(from) {
		return execution.AcceptanceJournal{}, fmt.Errorf("invalid acceptance journal request")
	}
	entries, err := journal.Load(reader.Path)
	if err != nil {
		return execution.AcceptanceJournal{}, fmt.Errorf("load retained acceptance journal: %w", err)
	}
	if len(entries) == 0 {
		// A retained run creates its journal before the first request. A
		// pre-write disconnect therefore has valid, empty evidence; bind the
		// decision to the canonical empty checkpoint instead of rejecting it.
		empty := sha256.Sum256(nil)
		return execution.AcceptanceJournal{RunID: runID, CheckpointHash: "sha256:" + hex.EncodeToString(empty[:])}, nil
	}
	if entries[0].RunID != runID {
		return execution.AcceptanceJournal{}, fmt.Errorf("acceptance journal run mismatch")
	}
	result := execution.AcceptanceJournal{RunID: runID, CheckpointHash: "sha256:" + entries[len(entries)-1].Hash}
	for _, entry := range entries {
		if entry.RunID != runID {
			return execution.AcceptanceJournal{}, fmt.Errorf("acceptance journal contains mixed runs")
		}
		if entry.TargetID != targetID || (entry.Kind != journal.RequestReceived && entry.Kind != journal.UpdateAccepted) {
			continue
		}
		// A bounded reconciliation may use only timestamped, hash-bound records
		// from its attempt window. Legacy records remain readable but cannot
		// establish acceptance or absence for a new reconciliation decision.
		if entry.RecordedAt.IsZero() {
			return execution.AcceptanceJournal{}, fmt.Errorf("acceptance journal entry lacks bounded timestamp")
		}
		recordedAt := entry.RecordedAt.UTC()
		if recordedAt.Before(from.UTC()) || recordedAt.After(until.UTC()) {
			continue
		}
		fingerprint := entry.Payload.Fingerprint
		if fingerprint != "" {
			fingerprint = "sha256:" + strings.TrimPrefix(fingerprint, "sha256:")
		}
		result.Entries = append(result.Entries, execution.AcceptanceEntry{AttemptID: entry.Payload.CorrelationID, RequestFingerprint: fingerprint, Received: entry.Kind == journal.RequestReceived, Accepted: entry.Kind == journal.UpdateAccepted, Sequence: int64(entry.Sequence)})
	}
	return result, nil
}

var _ execution.IndependentAcceptanceReader = Reader{}
