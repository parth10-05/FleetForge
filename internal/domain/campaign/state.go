package campaign

import "errors"

var ErrUnknownState = errors.New("unknown campaign state")

// State is the lifecycle state of a campaign aggregate.
type State string

const (
	StateDraft            State = "DRAFT"
	StateValidating       State = "VALIDATING"
	StateReady            State = "READY"
	StateRunningCanary    State = "RUNNING_CANARY"
	StateEvaluatingCanary State = "EVALUATING_CANARY"
	StateRunningWaves     State = "RUNNING_WAVES"
	StatePaused           State = "PAUSED"
	StateAborting         State = "ABORTING"
	StateCompleted        State = "COMPLETED"
	StateFailed           State = "FAILED"
	StateCancelled        State = "CANCELLED"
)

var allStates = []State{
	StateDraft,
	StateValidating,
	StateReady,
	StateRunningCanary,
	StateEvaluatingCanary,
	StateRunningWaves,
	StatePaused,
	StateAborting,
	StateCompleted,
	StateFailed,
	StateCancelled,
}

func ParseState(raw string) (State, error) {
	state := State(raw)
	for _, candidate := range allStates {
		if state == candidate {
			return state, nil
		}
	}
	return "", ErrUnknownState
}

func (state State) String() string { return string(state) }

func AllStates() []State { return append([]State(nil), allStates...) }
