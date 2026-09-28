package simulatorstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func testRunScenario() scenario.Scenario {
	return scenario.Scenario{SchemaVersion: scenario.SchemaVersionV1, ScenarioID: "retained-six-targets", Seed: "11", Topology: scenario.Topology{Sites: []string{"iad", "sfo"}}, Targets: []scenario.Target{
		{Index: 1, Site: "iad", Rack: "r1", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 2, Site: "iad", Rack: "r2", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 3, Site: "iad", Rack: "r2", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 4, Site: "sfo", Rack: "r3", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 5, Site: "sfo", Rack: "r3", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 6, Site: "sfo", Rack: "r4", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"},
	}, Timing: scenario.Timing{UploadMS: 1}, Limits: scenario.Limits{CallbackWorkers: 1, Subscriptions: 1, BodyBytes: 1024, JournalEntries: 1, OpenConnections: 1, EventsPerSecond: 1}}
}

func TestCreateRecoverAndOwnRun(t *testing.T) {
	run, err := CreateRun(t.TempDir(), testRunScenario())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{run.ManifestPath, run.ScenarioPath, run.SnapshotPath, run.JournalPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %q: %v", path, err)
		}
	}
	if _, err := RecoverRun(filepath.Dir(run.Root), run.ID); err != nil {
		t.Fatalf("RecoverRun() = %v", err)
	}
	release, err := run.AcquireOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.AcquireOwner(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("second owner error = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	// The metadata file remains, but OS ownership has been released and a new
	// live owner may acquire the run without treating the old file as a lock.
	if _, err := os.Stat(run.OwnerPath); err != nil {
		t.Fatalf("missing diagnostic owner metadata: %v", err)
	}
	if release, err := run.AcquireOwner(); err != nil {
		t.Fatal(err)
	} else if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryInvalidatesLeadingSnapshot(t *testing.T) {
	run, err := CreateRun(t.TempDir(), testRunScenario())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(run.SnapshotPath, []byte(`{"schema_version":"fleetforge.simulator.journal/v1","sequence":1,"hash":"missing"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverRun(filepath.Dir(run.Root), run.ID); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("RecoverRun() error = %v", err)
	}
	if _, err := os.Stat(run.InvalidPath); err != nil {
		t.Fatalf("missing invalid marker: %v", err)
	}
}

func TestOpenOrCreateRunRecoversExistingScenarioRun(t *testing.T) {
	root := t.TempDir()
	first, err := OpenOrCreateRun(root, testRunScenario())
	if err != nil {
		t.Fatalf("OpenOrCreateRun() error = %v", err)
	}
	second, err := OpenOrCreateRun(root, testRunScenario())
	if err != nil {
		t.Fatalf("OpenOrCreateRun() recovery error = %v", err)
	}
	if first.ID != second.ID || first.Root != second.Root {
		t.Fatalf("OpenOrCreateRun() = %#v, want recovered %#v", second, first)
	}
}

func TestRunWritesSnapshotBoundToJournalCheckpoint(t *testing.T) {
	run, err := CreateRun(t.TempDir(), testRunScenario())
	if err != nil {
		t.Fatal(err)
	}
	if err := run.WriteStateSnapshot(7, []byte(`{"targets":[]}`)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := journal.ReadSnapshot(run.SnapshotPath)
	if err != nil || snapshot.LogicalTime != 7 || snapshot.StateDigest == "" {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
}
