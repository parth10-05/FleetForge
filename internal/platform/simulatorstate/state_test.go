package simulatorstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRecoverableCreatesNewPair(t *testing.T) {
	directory := t.TempDir()
	statePath := filepath.Join(directory, "state.json")
	journalPath := filepath.Join(directory, "journal.json")
	if err := EnsureRecoverable(statePath, journalPath); err != nil {
		t.Fatalf("EnsureRecoverable() error = %v", err)
	}
	for _, path := range []string{statePath, journalPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("created file %q: %v", path, err)
		}
	}
}
func TestEnsureRecoverableRejectsPartialRetainedRun(t *testing.T) {
	directory := t.TempDir()
	statePath := filepath.Join(directory, "state.json")
	journalPath := filepath.Join(directory, "journal.json")
	if err := os.WriteFile(statePath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := EnsureRecoverable(statePath, journalPath)
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("EnsureRecoverable() error = %v, want incomplete retained run", err)
	}
}
func TestEnsureRecoverableRejectsCorruptJournal(t *testing.T) {
	directory := t.TempDir()
	statePath := filepath.Join(directory, "state.json")
	journalPath := filepath.Join(directory, "journal.json")
	if err := os.WriteFile(statePath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := EnsureRecoverable(statePath, journalPath)
	if err == nil || !strings.Contains(err.Error(), "recover simulator journal") {
		t.Fatalf("EnsureRecoverable() error = %v, want corrupt journal error", err)
	}
}
