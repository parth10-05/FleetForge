package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

// SnapshotBytes obtains an immutable engine projection and serializes it for a
// durable snapshot caller. The core package never writes the resulting bytes.
func SnapshotBytes(ctx context.Context, engine *Engine) ([]byte, Result, error) {
	result, err := engine.Submit(ctx, Snapshot{})
	if err != nil {
		return nil, Result{}, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, Result{}, err
	}
	return raw, result, nil
}

// RecoveredAcceptance is the trusted core representation reconstructed from a
// validated durable acceptance record. It deliberately has no artifact bytes.
type RecoveredAcceptance struct {
	Operation OperationState
	Task      TaskState
}

// StartRecovered starts an engine only after the journal acceptance projection
// has been fully validated and converted to deterministic core values.
func StartRecovered(input scenario.Scenario, entries []journal.Entry, queueSize int) (*Engine, error) {
	recovered, err := RecoverAcceptedOperations(input, entries)
	if err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	m := model{input: input, targets: map[string]target{}, operations: map[string]OperationState{}, tasks: map[string]TaskState{}, subscriptions: map[string]SubscriptionState{}, events: map[string]EventState{}, faults: map[string]FaultState{}}
	for _, value := range input.Targets {
		id, err := scenario.ID(input, "target", value.Index)
		if err != nil {
			return nil, err
		}
		m.targets[id] = target{TargetState: TargetState{ID: id, Site: value.Site, Rack: value.Rack, Version: value.InitialVersion, Healthy: true, Reachable: true, Phase: LifecycleHealthy}, ExpectedVersion: value.ExpectedVersion}
	}
	for _, value := range input.Faults {
		m.faults[value.FaultID] = FaultState{ID: value.FaultID, Name: value.Name, Scope: value.Scope, ScopeKey: value.ScopeKey, Remaining: value.Consumption, Priority: value.Priority}
	}
	for _, value := range recovered {
		m.operations[value.Operation.ID] = value.Operation
		m.tasks[value.Task.ID] = value.Task
		m.acceptanceOrdinal = value.Operation.Ordinal
		t := m.targets[value.Operation.TargetID]
		t.RequestCount++
		t.AcceptedCount++
		t.StagedVersion = value.Operation.Version
		m.targets[t.ID] = t
	}
	return startModel(m, queueSize)
}

// RecoverAcceptedOperations projects durable acceptance truth into the same
// deterministic operation/task identities used by the engine. It is pure: the
// caller must separately start an engine from this recovered state.
func RecoverAcceptedOperations(input scenario.Scenario, entries []journal.Entry) ([]RecoveredAcceptance, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	accepted, err := journal.ReplayAccepted(entries)
	if err != nil {
		return nil, err
	}
	targets := map[string]scenario.Target{}
	for _, target := range input.Targets {
		id, err := scenario.ID(input, "target", target.Index)
		if err != nil {
			return nil, err
		}
		targets[id] = target
	}
	recovered := make([]RecoveredAcceptance, 0, len(accepted))
	for index, entry := range accepted {
		target, found := targets[entry.TargetID]
		if !found {
			return nil, fmt.Errorf("%w: recovered acceptance target", ErrUnknownTarget)
		}
		ordinal := index + 1
		operationID, err := scenario.ID(input, "operation", ordinal)
		if err != nil {
			return nil, err
		}
		taskID, err := scenario.ID(input, "task", ordinal)
		if err != nil {
			return nil, err
		}
		recovered = append(recovered, RecoveredAcceptance{
			Operation: OperationState{ID: operationID, TargetID: entry.TargetID, AttemptID: entry.Payload.CorrelationID, Version: target.ExpectedVersion, TaskID: taskID, Ordinal: uint64(ordinal), State: LifecycleAccepted},
			Task:      TaskState{ID: taskID, TargetID: entry.TargetID, OperationID: operationID, State: LifecycleAccepted, Visible: true},
		})
	}
	sort.Slice(recovered, func(i, j int) bool { return recovered[i].Operation.Ordinal < recovered[j].Operation.Ordinal })
	return recovered, nil
}
