package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

const SnapshotSchemaVersion = "fleetforge.simulator.core-snapshot/v1"

type PersistedSnapshot struct {
	SchemaVersion string `json:"schema_version"`
	Result        Result `json:"result"`
}

// SnapshotBytes obtains an immutable engine projection and serializes it for a
// durable snapshot caller. The core package never writes the resulting bytes.
func SnapshotBytes(ctx context.Context, engine *Engine) ([]byte, Result, error) {
	result, err := engine.Submit(ctx, Snapshot{})
	if err != nil {
		return nil, Result{}, err
	}
	raw, err := json.Marshal(PersistedSnapshot{SchemaVersion: SnapshotSchemaVersion, Result: result})
	if err != nil {
		return nil, Result{}, err
	}
	return raw, result, nil
}

func DecodeSnapshot(raw []byte) (Result, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot PersistedSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Result{}, fmt.Errorf("invalid core snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Result{}, fmt.Errorf("invalid core snapshot")
	}
	if snapshot.SchemaVersion != SnapshotSchemaVersion {
		return Result{}, fmt.Errorf("invalid core snapshot")
	}
	return snapshot.Result, nil
}

// StartFromSnapshot restores immutable state only after decoding its versioned
// envelope and validating every target against the scenario.
func StartFromSnapshot(input scenario.Scenario, raw []byte, queueSize int) (*Engine, error) {
	result, err := DecodeSnapshot(raw)
	if err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	m := model{input: input, targets: map[string]target{}, operations: map[string]OperationState{}, tasks: map[string]TaskState{}, subscriptions: map[string]SubscriptionState{}, events: map[string]EventState{}, faults: map[string]FaultState{}}
	m.logicalTime = result.Time
	m.commandSequence = result.Sequence
	expected := map[string]scenario.Target{}
	for _, value := range input.Targets {
		id, err := scenario.ID(input, "target", value.Index)
		if err != nil {
			return nil, err
		}
		expected[id] = value
	}
	if len(result.State) != len(expected) {
		return nil, fmt.Errorf("invalid core snapshot")
	}
	for _, state := range result.State {
		value, found := expected[state.ID]
		if !found || state.Site != value.Site || state.Rack != value.Rack {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		if _, exists := m.targets[state.ID]; exists {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		m.targets[state.ID] = target{TargetState: state, ExpectedVersion: value.ExpectedVersion}
	}
	for _, value := range result.Operations {
		if _, ok := m.targets[value.TargetID]; !ok || value.ID == "" {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		if _, exists := m.operations[value.ID]; exists {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		m.operations[value.ID] = value
		if value.Ordinal > m.acceptanceOrdinal {
			m.acceptanceOrdinal = value.Ordinal
		}
	}
	for _, value := range result.Tasks {
		operation, ok := m.operations[value.OperationID]
		if !ok || value.ID == "" || value.TargetID != operation.TargetID || operation.TaskID != value.ID {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		if _, exists := m.tasks[value.ID]; exists {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		m.tasks[value.ID] = value
	}
	for _, value := range m.operations {
		task, ok := m.tasks[value.TaskID]
		if !ok || task.OperationID != value.ID || task.TargetID != value.TargetID {
			return nil, fmt.Errorf("invalid core snapshot")
		}
	}
	for _, value := range result.Subscriptions {
		if _, ok := m.targets[value.TargetID]; !ok || value.ID == "" {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		if _, exists := m.subscriptions[value.ID]; exists {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		m.subscriptions[value.ID] = value
		target := m.targets[value.TargetID]
		target.subscriptions++
		if target.subscriptions > uint64(input.Limits.Subscriptions) {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		m.targets[value.TargetID] = target
		if value.Ordinal > m.subscriptionOrdinal {
			m.subscriptionOrdinal = value.Ordinal
		}
	}
	for _, value := range result.Events {
		task, ok := m.tasks[value.TaskID]
		if !ok || value.ID == "" || value.TargetID != task.TargetID || value.Sequence == 0 {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		if _, exists := m.events[value.ID]; exists {
			return nil, fmt.Errorf("invalid core snapshot")
		}
		m.events[value.ID] = value
	}
	if err := m.restoreFaults(result.Faults); err != nil {
		return nil, err
	}
	return startModel(m, queueSize)
}

func (m *model) restoreFaults(states []FaultState) error {
	if len(states) != len(m.input.Faults) {
		return fmt.Errorf("invalid core snapshot")
	}
	expected := make(map[string]scenario.Fault, len(m.input.Faults))
	for _, value := range m.input.Faults {
		expected[value.FaultID] = value
	}
	for _, state := range states {
		value, ok := expected[state.ID]
		if !ok || state.Name != value.Name || state.Scope != value.Scope || state.ScopeKey != value.ScopeKey || state.Priority != value.Priority || state.Remaining < 0 || state.Remaining > value.Consumption {
			return fmt.Errorf("invalid core snapshot")
		}
		if _, exists := m.faults[state.ID]; exists {
			return fmt.Errorf("invalid core snapshot")
		}
		m.faults[state.ID] = state
	}
	return nil
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
