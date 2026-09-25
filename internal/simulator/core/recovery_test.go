package core

import (
	"context"
	"encoding/json"
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

func TestSnapshotBytesReturnsSerializableImmutableState(t *testing.T) {
	engine, err := Start(testScenario(), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	raw, result, err := SnapshotBytes(context.Background(), engine)
	if err != nil || len(raw) == 0 || len(result.State) != len(testScenario().Targets) {
		t.Fatalf("snapshot bytes = %q, %#v, %v", raw, result, err)
	}
}

func TestDecodeSnapshotRejectsUnknownAndIncompatibleShapes(t *testing.T) {
	if _, err := DecodeSnapshot([]byte(`{"schema_version":"unknown","result":{}}`)); err == nil {
		t.Fatal("incompatible snapshot was accepted")
	}
	if _, err := DecodeSnapshot([]byte(`{"schema_version":"fleetforge.simulator.core-snapshot/v1","result":{},"unexpected":true}`)); err == nil {
		t.Fatal("unknown snapshot field was accepted")
	}
}

func TestStartFromSnapshotRestoresImmutableProjection(t *testing.T) {
	input := testScenario()
	source, err := Start(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Submit(context.Background(), AdvanceTo{Time: 7}); err != nil {
		t.Fatal(err)
	}
	raw, before, err := SnapshotBytes(context.Background(), source)
	source.Close()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := StartFromSnapshot(input, raw, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	after, err := restored.Submit(context.Background(), Snapshot{})
	if err != nil || len(after.State) != len(before.State) || len(after.Tasks) != len(before.Tasks) || after.Time != before.Time || after.Sequence <= before.Sequence {
		t.Fatalf("restored snapshot = %#v, %v", after, err)
	}
}

func TestStartFromSnapshotRejectsDuplicateOperationID(t *testing.T) {
	input := testScenario()
	state := make([]TargetState, 0, len(input.Targets))
	for _, value := range input.Targets {
		id, err := scenario.ID(input, "target", value.Index)
		if err != nil {
			t.Fatal(err)
		}
		state = append(state, TargetState{ID: id, Site: value.Site, Rack: value.Rack, Version: value.InitialVersion, Healthy: true, Reachable: true, Phase: LifecycleHealthy})
	}
	result := Result{State: state, Operations: []OperationState{{ID: "op-1", TargetID: state[0].ID}, {ID: "op-1", TargetID: state[0].ID}}}
	raw, err := json.Marshal(PersistedSnapshot{SchemaVersion: SnapshotSchemaVersion, Result: result})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StartFromSnapshot(input, raw, 4); err == nil {
		t.Fatal("duplicate operation ID was accepted")
	}
}

func TestStartFromSnapshotRetainsSubscriptionCapacity(t *testing.T) {
	input := testScenario()
	source, err := Start(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	target, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Submit(context.Background(), CreateSubscription{TargetID: target, Fingerprint: "callback-1"}); err != nil {
		t.Fatal(err)
	}
	raw, _, err := SnapshotBytes(context.Background(), source)
	source.Close()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := StartFromSnapshot(input, raw, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err := restored.Submit(context.Background(), CreateSubscription{TargetID: target, Fingerprint: "callback-2"}); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("subscription limit error = %v", err)
	}
}

func TestStartFromSnapshotRetainsFaultConsumption(t *testing.T) {
	input := testScenario()
	input.Faults = []scenario.Fault{{FaultID: "fault-1", Name: "RF-F01", Scope: "rack", ScopeKey: "r1", Trigger: scenario.Trigger{Kind: "logical_time", Value: 0}, Consumption: 2}}
	target, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	source, err := Start(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Submit(context.Background(), ConsumeFault{TargetID: target, FaultID: "fault-1"}); err != nil {
		t.Fatal(err)
	}
	raw, _, err := SnapshotBytes(context.Background(), source)
	source.Close()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := StartFromSnapshot(input, raw, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err := restored.Submit(context.Background(), ConsumeFault{TargetID: target, FaultID: "fault-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.Submit(context.Background(), ConsumeFault{TargetID: target, FaultID: "fault-1"}); !errors.Is(err, ErrFaultExhausted) {
		t.Fatalf("fault exhaustion error = %v", err)
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
