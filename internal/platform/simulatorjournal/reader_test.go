package simulatorjournal

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/journal"
)

func TestReaderReturnsValidatedAcceptanceEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := journal.OpenWriter(path, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Append(journal.Record{Kind: journal.RequestReceived, TargetID: "node-a", Payload: journal.Payload{CorrelationID: "attempt-1", Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Append(journal.Record{Kind: journal.UpdateAccepted, TargetID: "node-a", Payload: journal.Payload{CorrelationID: "attempt-1", Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := (Reader{Path: path}).ReadAcceptance(context.Background(), "run-1", "node-a", time.Now().Add(-time.Minute), time.Now())
	if err != nil || len(result.Entries) != 2 || !result.Entries[0].Received || !result.Entries[1].Accepted || result.CheckpointHash == "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestReaderRejectsRunMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := journal.OpenWriter(path, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Append(journal.Record{Kind: journal.Diagnostic}); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	if _, err := (Reader{Path: path}).ReadAcceptance(context.Background(), "other-run", "node-a", time.Now().Add(-time.Minute), time.Now()); err == nil {
		t.Fatal("run mismatch was accepted")
	}
}

func TestReaderFiltersAcceptanceOutsideAttemptWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := journal.OpenWriter(path, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, at := range []time.Time{now.Add(-time.Minute), now, now.Add(time.Minute)} {
		if _, err := writer.Append(journal.Record{RecordedAt: at, Kind: journal.RequestReceived, TargetID: "node-a", Payload: journal.Payload{CorrelationID: "attempt-1", Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := (Reader{Path: path}).ReadAcceptance(context.Background(), "run-1", "node-a", now, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 || result.Entries[0].Sequence != 2 {
		t.Fatalf("bounded entries=%+v", result.Entries)
	}
}
