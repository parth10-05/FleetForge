package core

import (
	"context"
	"errors"
	"testing"

	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func TestRecoverAcceptedOperationsReconstructsEachDurableAcceptance(t *testing.T) {
	input := testScenario()
	target, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	first, err := journal.New("run-1", 1, 0, journal.UpdateAccepted, target, "same-attempt", "digest", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := journal.New("run-1", 2, 0, journal.UpdateAccepted, target, "same-attempt", "digest", first.Hash)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := RecoverAcceptedOperations(input, []journal.Entry{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 2 || recovered[0].Operation.ID == recovered[1].Operation.ID || recovered[0].Task.ID == recovered[1].Task.ID {
		t.Fatalf("recovered acceptances = %#v", recovered)
	}
	if recovered[0].Operation.AttemptID != "same-attempt" || recovered[1].Operation.Version != "2" {
		t.Fatalf("recovered operation metadata = %#v", recovered)
	}
}

func TestStartRecoveredExposesDurableAcceptedTruth(t *testing.T) {
	input := testScenario()
	target, _ := scenario.ID(input, "target", 1)
	entry, err := journal.New("run-1", 1, 0, journal.UpdateAccepted, target, "attempt", "digest", "")
	if err != nil {
		t.Fatal(err)
	}
	engine, err := StartRecovered(input, []journal.Entry{entry}, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	result, err := engine.Submit(context.Background(), Observe{TargetID: target})
	accepted := uint64(0)
	for _, state := range result.State {
		if state.ID == target {
			accepted = state.AcceptedCount
		}
	}
	if err != nil || len(result.Operations) != 1 || len(result.Tasks) != 1 || accepted != 1 {
		t.Fatalf("recovered result = %#v, %v", result, err)
	}
}

func TestSnapshotIsReadOnlyCompleteProjection(t *testing.T) {
	engine, err := Start(testScenario(), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	first, err := engine.Submit(context.Background(), Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Submit(context.Background(), Snapshot{})
	if err != nil || len(first.State) != len(second.State) || first.Sequence+1 != second.Sequence {
		t.Fatalf("snapshot results = %#v, %#v, %v", first, second, err)
	}
}

func TestRecoverAcceptedOperationsRejectsUnknownTarget(t *testing.T) {
	input := testScenario()
	entry, err := journal.New("run-1", 1, 0, journal.UpdateAccepted, "target-not-in-scenario", "attempt", "digest", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverAcceptedOperations(input, []journal.Entry{entry}); !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("unknown target error = %v", err)
	}
}
