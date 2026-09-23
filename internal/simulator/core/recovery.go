package core

import (
	"fmt"
	"sort"

	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

// RecoveredAcceptance is the trusted core representation reconstructed from a
// validated durable acceptance record. It deliberately has no artifact bytes.
type RecoveredAcceptance struct {
	Operation OperationState
	Task      TaskState
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
