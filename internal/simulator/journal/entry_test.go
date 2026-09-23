package journal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHashChainRejectsGapsAndTampering(t *testing.T) {
	first, err := New("run-1", 1, 0, RequestReceived, "target-1", "request-1", "sha256-abc", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := New("run-1", 2, 1, UpdateAccepted, "target-1", "operation-1", "sha256-abc", first.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate([]Entry{first, second}); err != nil {
		t.Fatal(err)
	}
	tampered := second
	tampered.Kind = "different"
	if err := Validate([]Entry{first, tampered}); !errors.Is(err, ErrInvalidChain) {
		t.Fatalf("tampering error = %v", err)
	}
	gap := second
	gap.Sequence = 3
	if err := Validate([]Entry{first, gap}); !errors.Is(err, ErrInvalidChain) {
		t.Fatalf("gap error = %v", err)
	}
}

func TestAppendAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	first, _ := New("run-1", 1, 0, RequestReceived, "target-1", "request-1", "sha256-abc", "")
	second, _ := New("run-1", 2, 1, UpdateAccepted, "target-1", "operation-1", "sha256-abc", first.Hash)
	if err := Append(path, first); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, second); err != nil {
		t.Fatal(err)
	}
	entries, err := Load(path)
	if err != nil || len(entries) != 2 {
		t.Fatalf("Load() = %#v, %v", entries, err)
	}
	if err := os.WriteFile(path, []byte("{bad}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); !errors.Is(err, ErrInvalidChain) {
		t.Fatalf("corrupt journal error = %v", err)
	}
}

func TestWriterSerializesAndSyncsDurableKinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := OpenWriter(path, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	for index := 0; index < 10_000; index++ {
		kind := Diagnostic
		if index == 0 {
			kind = RequestReceived
		}
		if index == 1 {
			kind = UpdateAccepted
		}
		entry, err := writer.Append(Record{LogicalTime: 1, Kind: kind, TargetID: "target-1", Payload: Payload{Fingerprint: "digest"}})
		if err != nil {
			t.Fatalf("append %d: %v", index, err)
		}
		if entry.Sequence != uint64(index+1) {
			t.Fatalf("sequence = %d", entry.Sequence)
		}
	}
	if writer.SyncCount() != 2 {
		t.Fatalf("durable sync count = %d", writer.SyncCount())
	}
	entries, err := Load(path)
	if err != nil || len(entries) != 10_000 {
		t.Fatalf("load = %d, %v", len(entries), err)
	}
}

func TestTypedRecordsRejectUnsafePayloadAndPartialFinalLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := OpenWriter(path, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Append(Record{Kind: RequestReceived, Payload: Payload{Detail: "password=not-allowed"}}); !errors.Is(err, ErrInvalidChain) {
		t.Fatalf("unsafe payload error = %v", err)
	}
	if _, err := writer.Append(Record{Kind: Kind("unknown"), Payload: Payload{Fingerprint: "x"}}); !errors.Is(err, ErrInvalidChain) {
		t.Fatalf("unknown kind error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); !errors.Is(err, ErrInvalidChain) {
		t.Fatalf("partial line error = %v", err)
	}
}
