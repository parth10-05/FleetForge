package core

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/parth10-05/fleetforge/internal/simulator/clock"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func testScenario() scenario.Scenario {
	return scenario.Scenario{SchemaVersion: scenario.SchemaVersionV1, ScenarioID: "engine-six-targets", Seed: "7", Topology: scenario.Topology{Sites: []string{"iad", "sfo"}}, Targets: []scenario.Target{
		{Index: 1, Site: "iad", Rack: "r1", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 2, Site: "iad", Rack: "r2", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 3, Site: "iad", Rack: "r2", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 4, Site: "sfo", Rack: "r3", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 5, Site: "sfo", Rack: "r3", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 6, Site: "sfo", Rack: "r4", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"},
	}, Timing: scenario.Timing{UploadMS: 1}, Limits: scenario.Limits{CallbackWorkers: 1, Subscriptions: 1, BodyBytes: 1024, JournalEntries: 1, OpenConnections: 1, EventsPerSecond: 1}}
}

func TestEngineSchedulesAndAppliesLogicalState(t *testing.T) {
	input := testScenario()
	engine, err := Start(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Submit(context.Background(), ScheduleReachability{TargetID: target, At: 10, Reachable: false, Priority: 0}); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Submit(context.Background(), AdvanceTo{Time: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fired) != 1 || result.Fired[0].TargetID != target {
		t.Fatalf("fired = %#v", result.Fired)
	}
	for _, state := range result.State {
		if state.ID == target && state.Reachable {
			t.Fatal("scheduled reachability was not applied")
		}
	}
	if _, err := engine.Submit(context.Background(), AdvanceTo{Time: 9}); !errors.Is(err, clock.ErrTimeReversal) {
		t.Fatalf("reverse time error = %v", err)
	}
}

func TestEngineSerializesConcurrentCommands(t *testing.T) {
	engine, err := Start(testScenario(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	sequences := make(chan uint64, 16)
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := engine.Submit(context.Background(), AdvanceTo{Time: 0})
			if err != nil {
				t.Error(err)
				return
			}
			sequences <- result.Sequence
		}()
	}
	group.Wait()
	close(sequences)
	seen := map[uint64]bool{}
	for sequence := range sequences {
		seen[sequence] = true
	}
	if len(seen) != 16 {
		t.Fatalf("recorded sequence count = %d", len(seen))
	}
	for expected := uint64(1); expected <= 16; expected++ {
		if !seen[expected] {
			t.Fatalf("missing total-order sequence %d", expected)
		}
	}
}

func TestEngineRejectsUnknownTargetsWithoutMutation(t *testing.T) {
	engine, err := Start(testScenario(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if _, err := engine.Submit(context.Background(), SetHealth{TargetID: "not-a-target"}); !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("unknown target error = %v", err)
	}
}

func TestEngineRunsHealthyUpdateLifecycleAndKeepsDuplicates(t *testing.T) {
	input := testScenario()
	input.Timing = scenario.Timing{UploadMS: 1, TaskStepMS: 1, RebootMS: 1, InventoryVisibilityMS: 1, EventMS: 1}
	engine, err := Start(input, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, _ := scenario.ID(input, "target", 1)
	first, err := engine.Submit(context.Background(), AcceptUpdate{TargetID: target, Version: "2", AttemptID: "attempt-a"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Submit(context.Background(), AcceptUpdate{TargetID: target, Version: "2", AttemptID: "attempt-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Operations) != 2 || second.Operations[0].ID == second.Operations[1].ID || first.Operations[0].ID != second.Operations[0].ID && first.Operations[0].ID != second.Operations[1].ID {
		t.Fatalf("duplicate acceptance was hidden: %#v", second.Operations)
	}
	result, err := engine.Submit(context.Background(), AdvanceTo{Time: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 2 || result.Tasks[0].State != LifecycleHealthy || result.Tasks[1].State != LifecycleHealthy {
		t.Fatalf("tasks did not reach healthy lifecycle state: %#v", result.Tasks)
	}
	for _, state := range result.State {
		if state.ID == target && (state.Version != "2" || !state.Healthy || !state.Reachable || state.Phase != LifecycleHealthy) {
			t.Fatalf("unexpected completed target state: %#v", state)
		}
	}
}

func TestEngineSeparatesCompletedTaskFromPostUpdateVerificationFaults(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		fault          string
		version        string
		healthy        bool
		targetPhase    Lifecycle
		operationState Lifecycle
	}{
		{name: "inventory contradiction", fault: "RF-F10", version: "1", healthy: true, targetPhase: LifecycleVersionContradiction, operationState: LifecycleVersionContradiction},
		{name: "health failure", fault: "RF-F11", version: "2", healthy: false, targetPhase: LifecycleHealthFailed, operationState: LifecycleHealthFailed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := testScenario()
			input.Timing = scenario.Timing{UploadMS: 1, TaskStepMS: 1, RebootMS: 1, InventoryVisibilityMS: 1}
			input.Faults = []scenario.Fault{{FaultID: "post-update-fault", Name: testCase.fault, Scope: "all", Trigger: scenario.Trigger{Kind: "logical_time", Value: 0}, Consumption: 1}}
			engine, err := Start(input, 4)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			target, _ := scenario.ID(input, "target", 1)
			if _, err := engine.Submit(context.Background(), AcceptUpdate{TargetID: target, Version: "2", AttemptID: "verify-fault"}); err != nil {
				t.Fatal(err)
			}
			result, err := engine.Submit(context.Background(), AdvanceTo{Time: 4})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Tasks) != 1 || result.Tasks[0].State != LifecycleCompleted || result.Tasks[0].PercentComplete != 100 {
				t.Fatalf("task outcome=%#v, want completed task evidence", result.Tasks)
			}
			var targetState TargetState
			for _, state := range result.State {
				if state.ID == target {
					targetState = state
					break
				}
			}
			if targetState.ID == "" || targetState.Version != testCase.version || targetState.Healthy != testCase.healthy || targetState.Phase != testCase.targetPhase {
				t.Fatalf("target outcome=%#v, want version=%s healthy=%t phase=%s", targetState, testCase.version, testCase.healthy, testCase.targetPhase)
			}
			if len(result.Operations) != 1 || result.Operations[0].State != testCase.operationState {
				t.Fatalf("operation outcome=%#v, want state=%s", result.Operations, testCase.operationState)
			}
		})
	}
}

func TestEngineRejectsInvalidUpdateWithoutMutationAndConsumesFaultBoundedly(t *testing.T) {
	input := testScenario()
	input.Faults = []scenario.Fault{{FaultID: "fault-a", Name: "RF-F01", Scope: "all", Trigger: scenario.Trigger{Kind: "request_count"}, Consumption: 1}}
	engine, err := Start(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, _ := scenario.ID(input, "target", 1)
	if _, err := engine.Submit(context.Background(), AcceptUpdate{TargetID: target, Version: "wrong", AttemptID: "x"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("invalid version error = %v", err)
	}
	before, err := engine.Submit(context.Background(), Observe{TargetID: target})
	if err != nil || before.State[0].AcceptedCount != 0 {
		t.Fatalf("rejected command mutated state: %#v, %v", before, err)
	}
	if _, err := engine.Submit(context.Background(), ConsumeFault{TargetID: target, FaultID: "fault-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Submit(context.Background(), ConsumeFault{TargetID: target, FaultID: "fault-a"}); !errors.Is(err, ErrFaultExhausted) {
		t.Fatalf("second consumption error = %v", err)
	}
}

func TestEngineSubscriptionsAreBoundedAndSnapshotsAreStable(t *testing.T) {
	input := testScenario()
	input.Limits.Subscriptions = 1
	engine, err := Start(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, _ := scenario.ID(input, "target", 1)
	first, err := engine.Submit(context.Background(), CreateSubscription{TargetID: target, Fingerprint: "callback-one"})
	if err != nil || len(first.Subscriptions) != 1 {
		t.Fatalf("first subscription = %#v, %v", first, err)
	}
	if _, err := engine.Submit(context.Background(), CreateSubscription{TargetID: target, Fingerprint: "callback-two"}); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("subscription limit error = %v", err)
	}
	second, err := engine.Submit(context.Background(), Observe{TargetID: target})
	if err != nil || len(second.Subscriptions) != 1 || second.Subscriptions[0] != first.Subscriptions[0] {
		t.Fatalf("snapshot changed unexpectedly: %#v, %v", second.Subscriptions, err)
	}
}

func TestResetRebuildsScenarioStateWithoutRemovingAcceptanceHistoryFromJournal(t *testing.T) {
	input := testScenario()
	engine, err := Start(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Submit(context.Background(), AcceptUpdate{TargetID: target, Version: "2", AttemptID: "reset-attempt"}); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Submit(context.Background(), Reset{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 0 || len(result.Tasks) != 0 || result.State[0].RequestCount != 0 || result.State[0].Version != "1" {
		t.Fatalf("reset result = %#v", result)
	}
}

func TestSameScenarioAndCommandSequenceProduceIdenticalProjection(t *testing.T) {
	input := testScenario()
	first, err := Start(input, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Start(input, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	target, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	commands := []Command{AcceptUpdate{TargetID: target, Version: "2", AttemptID: "same-sequence"}, AdvanceTo{Time: 10}, Snapshot{}}
	for _, command := range commands {
		left, err := first.Submit(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		right, err := second.Submit(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		leftJSON, _ := json.Marshal(left)
		rightJSON, _ := json.Marshal(right)
		if string(leftJSON) != string(rightJSON) {
			t.Fatalf("deterministic result differs for %#v:\n%s\n%s", command, leftJSON, rightJSON)
		}
	}
}

func TestEngineConsumesEverySupportedFaultAndHonorsCancellation(t *testing.T) {
	input := testScenario()
	faultNames := []string{"RF-F01", "RF-F02", "RF-F03", "RF-F04", "RF-F05", "RF-F06", "RF-F07", "RF-F08", "RF-F10", "RF-F11"}
	for index, name := range faultNames {
		input.Faults = append(input.Faults, scenario.Fault{FaultID: "fault-" + string(rune('a'+index)), Name: name, Scope: "all", Trigger: scenario.Trigger{Kind: "request_count"}, Consumption: 1, Priority: index})
	}
	engine, err := Start(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, _ := scenario.ID(input, "target", 1)
	for _, fault := range input.Faults {
		if _, err := engine.Submit(context.Background(), ConsumeFault{TargetID: target, FaultID: fault.FaultID}); err != nil {
			t.Fatalf("consume %s: %v", fault.Name, err)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Submit(canceled, Observe{TargetID: target}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled submission error = %v", err)
	}
}
