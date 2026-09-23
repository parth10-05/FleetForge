package core

import (
	"context"
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
