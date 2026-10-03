// Package simulatorstate owns simulator state/journal bootstrap.
package simulatorstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EnsureRecoverable opens an existing pair only if both artifacts are valid.
// For a new run it creates both. A partial retained run fails closed.
func EnsureRecoverable(statePath, journalPath string) error {
	stateExists, err := regularFileExists(statePath)
	if err != nil {
		return fmt.Errorf("simulator state path: %w", err)
	}
	journalExists, err := regularFileExists(journalPath)
	if err != nil {
		return fmt.Errorf("simulator journal path: %w", err)
	}
	switch {
	case !stateExists && !journalExists:
		if err := writeNew(statePath, []byte("{}\n")); err != nil {
			return fmt.Errorf("create simulator state: %w", err)
		}
		if err := writeNew(journalPath, []byte("[]\n")); err != nil {
			return fmt.Errorf("create simulator journal: %w", err)
		}
		return nil
	case stateExists != journalExists:
		return fmt.Errorf("retained simulator run is incomplete: state and journal must both exist")
	}
	if err := validateObject(statePath); err != nil {
		return fmt.Errorf("recover simulator state: %w", err)
	}
	if err := validateArray(journalPath); err != nil {
		return fmt.Errorf("recover simulator journal: %w", err)
	}
	return nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("must be a regular file")
	}
	return true, nil
}
func writeNew(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o600)
}
func validateObject(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(contents, &value); err != nil {
		return fmt.Errorf("must contain a JSON object")
	}
	return nil
}
func validateArray(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var value []json.RawMessage
	if err := json.Unmarshal(contents, &value); err != nil {
		return fmt.Errorf("must contain a JSON array")
	}
	return nil
}
