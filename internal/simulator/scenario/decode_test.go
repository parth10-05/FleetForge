package scenario

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDecodeRejectsUnknownDuplicateAndTrailingJSON(t *testing.T) {
	raw, err := json.Marshal(validScenario())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(raw); err != nil {
		t.Fatalf("Decode(valid) = %v", err)
	}
	for name, input := range map[string]string{
		"unknown":   `{"schema_version":"fleetforge.simulator.scenario/v1","unknown":true}`,
		"duplicate": `{"schema_version":"fleetforge.simulator.scenario/v1","schema_version":"fleetforge.simulator.scenario/v1"}`,
		"trailing":  string(raw) + ` {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(input)); !errors.Is(err, ErrInvalidScenario) {
				t.Fatalf("Decode() error = %v", err)
			}
		})
	}
}

func TestGoldenFixtures(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	for _, name := range []string{"healthy.json", "lost-event.json", "post-accept-disconnect.json", "task-disappearance.json", "canary-health-failure.json", "controlled-ambiguity.json"} {
		valid, err := os.ReadFile(filepath.Join(root, "test", "fixtures", "simulator", "v1", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(valid); err != nil {
			t.Fatalf("Decode(%s) = %v", name, err)
		}
	}
	invalid, err := os.ReadFile(filepath.Join(root, "test", "fixtures", "simulator", "v1", "invalid-duplicate-key.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(invalid); !errors.Is(err, ErrInvalidScenario) {
		t.Fatalf("Decode(invalid fixture) error = %v", err)
	}
}

func FuzzDecode(f *testing.F) {
	raw, _ := json.Marshal(validScenario())
	f.Add(string(raw))
	f.Add(`{`)
	f.Add(`{"schema_version":"fleetforge.simulator.scenario/v1","schema_version":"x"}`)
	f.Fuzz(func(t *testing.T, input string) {
		_, err := Decode([]byte(input))
		if err != nil && len(err.Error()) > 128 {
			t.Fatalf("unbounded decode error: %d", len(err.Error()))
		}
	})
}
