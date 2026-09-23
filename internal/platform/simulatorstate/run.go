package simulatorstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

var ErrInvalidRun = errors.New("invalid retained simulator run")

type Run struct{ Root, ID, ManifestPath, ScenarioPath, SnapshotPath, JournalPath, OwnerPath, InvalidPath string }
type manifest struct {
	SchemaVersion  string `json:"schema_version"`
	RunID          string `json:"run_id"`
	ScenarioDigest string `json:"scenario_digest"`
}
type snapshot struct {
	SchemaVersion string `json:"schema_version"`
	Sequence      uint64 `json:"sequence"`
	Hash          string `json:"hash"`
}

func CreateRun(root string, input scenario.Scenario) (Run, error) {
	canonical, err := scenario.CanonicalBytes(input)
	if err != nil {
		return Run{}, err
	}
	digest, err := scenario.Digest(input)
	if err != nil {
		return Run{}, err
	}
	id, err := scenario.ID(input, "run", 0)
	if err != nil {
		return Run{}, err
	}
	run := paths(root, id)
	if err := os.MkdirAll(run.Root, 0o700); err != nil {
		return Run{}, err
	}
	if err := writeExclusive(run.ManifestPath, mustJSON(manifest{SchemaVersion: "fleetforge.simulator.run/v1", RunID: id, ScenarioDigest: digest})); err != nil {
		return Run{}, err
	}
	if err := writeExclusive(run.ScenarioPath, canonical); err != nil {
		return Run{}, err
	}
	if err := writeExclusive(run.JournalPath, nil); err != nil {
		return Run{}, err
	}
	if err := journal.WriteSnapshot(run.SnapshotPath, 0, ""); err != nil {
		return Run{}, err
	}
	return run, nil
}

func RecoverRun(root, id string) (Run, error) {
	run := paths(root, id)
	var record manifest
	if err := readJSON(run.ManifestPath, &record); err != nil || record.SchemaVersion != "fleetforge.simulator.run/v1" || record.RunID != id {
		return invalidate(run, "manifest")
	}
	raw, err := os.ReadFile(run.ScenarioPath)
	if err != nil {
		return invalidate(run, "scenario")
	}
	input, err := scenario.Decode(raw)
	if err != nil {
		return invalidate(run, "scenario")
	}
	digest, err := scenario.Digest(input)
	if err != nil || digest != record.ScenarioDigest {
		return invalidate(run, "scenario digest")
	}
	entries, err := journal.Load(run.JournalPath)
	if err != nil {
		return invalidate(run, "journal")
	}
	var checkpoint snapshot
	if err := readJSON(run.SnapshotPath, &checkpoint); err != nil || checkpoint.SchemaVersion != journal.SchemaVersionV1 || checkpoint.Sequence > uint64(len(entries)) {
		return invalidate(run, "snapshot")
	}
	if checkpoint.Sequence > 0 && entries[checkpoint.Sequence-1].Hash != checkpoint.Hash {
		return invalidate(run, "snapshot hash")
	}
	return run, nil
}

func (r Run) AcquireOwner() (func() error, error) {
	file, err := os.OpenFile(r.OwnerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%w: owner", ErrInvalidRun)
	}
	if _, err := file.WriteString("owned\n"); err != nil {
		file.Close()
		os.Remove(r.OwnerPath)
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return func() error { return os.Remove(r.OwnerPath) }, nil
}

func paths(root, id string) Run {
	base := filepath.Join(root, id)
	return Run{Root: base, ID: id, ManifestPath: filepath.Join(base, "manifest.json"), ScenarioPath: filepath.Join(base, "scenario.json"), SnapshotPath: filepath.Join(base, "snapshot.json"), JournalPath: filepath.Join(base, "journal.jsonl"), OwnerPath: filepath.Join(base, "owner.lock"), InvalidPath: filepath.Join(base, "invalid.json")}
}
func writeExclusive(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(contents)
	return err
}
func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
func readJSON(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}
func invalidate(run Run, reason string) (Run, error) {
	_ = os.WriteFile(run.InvalidPath, mustJSON(map[string]string{"reason": reason}), 0o600)
	return Run{}, fmt.Errorf("%w: %s", ErrInvalidRun, reason)
}
