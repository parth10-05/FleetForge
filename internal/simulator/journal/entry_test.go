package journal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHashChainRejectsGapsAndTampering(t *testing.T) {
	first, err := New("run-1", 1, 0, "received", "target-1", "request-1", "sha256:abc", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := New("run-1", 2, 1, "accepted", "target-1", "operation-1", "sha256:abc", first.Hash)
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
	first, _ := New("run-1", 1, 0, "received", "target-1", "request-1", "sha256:abc", "")
	second, _ := New("run-1", 2, 1, "accepted", "target-1", "operation-1", "sha256:abc", first.Hash)
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
