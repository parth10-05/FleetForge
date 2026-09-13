package campaign

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCatalogIsCartesianAndProtectsTransitionData(t *testing.T) {
	transitions := AllTransitions()
	if len(transitions) != len(AllStates())*len(AllCommands()) {
		t.Fatalf("transition count = %d", len(transitions))
	}
	seen := map[State]map[Command]bool{}
	for _, transition := range transitions {
		if seen[transition.From] == nil {
			seen[transition.From] = map[Command]bool{}
		}
		if seen[transition.From][transition.Command] {
			t.Fatalf("duplicate transition %s/%s", transition.From, transition.Command)
		}
		seen[transition.From][transition.Command] = true
		if transition.Class == TransitionAllowed && transition.Revision != RevisionIncrement {
			t.Fatalf("allowed %s/%s does not increment revision", transition.From, transition.Command)
		}
		if transition.Class != TransitionAllowed && transition.Revision != RevisionUnchanged {
			t.Fatalf("non-allowed %s/%s changes revision", transition.From, transition.Command)
		}
	}
	pause, _ := TransitionFor(StateRunningCanary, CommandPause)
	if pause.Epoch != EpochIncrement || pause.Suspended != SuspendedPhaseCapture {
		t.Fatal("pause contract does not fence epoch and capture phase")
	}
	resume, _ := TransitionFor(StatePaused, CommandResume)
	if resume.Resolution != StateResolutionRestoreSuspended || resume.Suspended != SuspendedPhaseRestore {
		t.Fatal("resume contract does not restore suspended phase")
	}
	copy := pause.Guards
	copy[0] = "mutated"
	again, _ := TransitionFor(StateRunningCanary, CommandPause)
	if again.Guards[0] == "mutated" {
		t.Fatal("catalog exposed mutable guard storage")
	}
}

func TestFixtureMatchesCatalogAxesAndOverrides(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "test", "fixtures", "domain", "v1", "campaign-transitions.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version   string   `json:"version"`
		States    []string `json:"states"`
		Commands  []string `json:"commands"`
		Overrides []struct {
			From    string `json:"from"`
			Command string `json:"command"`
			Class   string `json:"class"`
			Result  string `json:"result"`
		} `json:"overrides"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != "v1" || len(fixture.States) != len(AllStates()) || len(fixture.Commands) != len(AllCommands()) {
		t.Fatal("fixture axes drifted")
	}
	for _, row := range fixture.Overrides {
		state, err := ParseState(row.From)
		if err != nil {
			t.Fatal(err)
		}
		command, err := ParseCommand(row.Command)
		if err != nil {
			t.Fatal(err)
		}
		actual, ok := TransitionFor(state, command)
		if !ok || actual.Class.String() != row.Class || actual.Result.String() != row.Result {
			t.Fatalf("fixture mismatch for %s/%s", row.From, row.Command)
		}
	}
}
