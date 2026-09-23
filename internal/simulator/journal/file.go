package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func Append(path string, entry Entry) error {
	entries, err := Load(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(entries) == 0 {
		if entry.Sequence != 1 || entry.PreviousHash != "" {
			return ErrInvalidChain
		}
	} else {
		last := entries[len(entries)-1]
		if entry.Sequence != last.Sequence+1 || entry.PreviousHash != last.Hash {
			return ErrInvalidChain
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func Load(path string) ([]Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeLines(raw)
}

func WriteSnapshot(path string, sequence uint64, hash string) error {
	if (sequence == 0 && hash != "") || (sequence > 0 && hash == "") {
		return fmt.Errorf("invalid simulator snapshot")
	}
	raw, err := json.Marshal(struct {
		SchemaVersion string `json:"schema_version"`
		Sequence      uint64 `json:"sequence"`
		Hash          string `json:"hash"`
	}{SchemaVersion: SchemaVersionV1, Sequence: sequence, Hash: hash})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
