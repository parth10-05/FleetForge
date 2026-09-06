package targetupdate

import "errors"

var ErrUnknownState = errors.New("unknown target update state")

// State is the lifecycle state of a target-update aggregate.
type State string

const (
	StatePending        State = "PENDING"
	StatePrecheck       State = "PRECHECK"
	StateSubmitting     State = "SUBMITTING"
	StateUpdateAccepted State = "UPDATE_ACCEPTED"
	StateWaitingReboot  State = "WAITING_REBOOT"
	StateVerifying      State = "VERIFYING"
	StateRetryWait      State = "RETRY_WAIT"
	StateNeedsAttention State = "NEEDS_ATTENTION"
	StateSucceeded      State = "SUCCEEDED"
	StateFailedTerminal State = "FAILED_TERMINAL"
	StateSkipped        State = "SKIPPED"
	StateCancelled      State = "CANCELLED"
)

var allStates = []State{
	StatePending,
	StatePrecheck,
	StateSubmitting,
	StateUpdateAccepted,
	StateWaitingReboot,
	StateVerifying,
	StateRetryWait,
	StateNeedsAttention,
	StateSucceeded,
	StateFailedTerminal,
	StateSkipped,
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
