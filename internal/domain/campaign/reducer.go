package campaign

import (
	"github.com/parth10-05/fleetforge/internal/domain/reason"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

type Aggregate struct {
	State          State
	Revision       value.Revision
	CommandEpoch   value.CommandEpoch
	SuspendedPhase State
}
type CommandInput struct {
	ID               value.CommandID
	Command          Command
	ExpectedRevision value.Revision
	ExpectedEpoch    value.CommandEpoch
	Duplicate        bool
	GuardsSatisfied  bool
}
type DecisionClass string

const (
	DecisionAllowed         DecisionClass = "allowed"
	DecisionConflict        DecisionClass = "conflict"
	DecisionInvalidInput    DecisionClass = "invalid_input"
	DecisionUnsafeCondition DecisionClass = "unsafe_condition"
	DecisionNoOp            DecisionClass = "no_op"
)

type Decision struct {
	Class      DecisionClass
	Aggregate  Aggregate
	Reason     reason.Code
	Events     []string
	NextIntent string
}

func Reduce(aggregate Aggregate, input CommandInput) Decision {
	if input.ID.IsZero() || !knownState(aggregate.State) || !knownCommand(input.Command) || aggregate.SuspendedPhase != "" && !isSuspensible(aggregate.SuspendedPhase) {
		return reject(DecisionInvalidInput, aggregate, reason.CodeInternalInvariantInvalidAggregate)
	}
	if input.Duplicate {
		return reject(DecisionNoOp, aggregate, reason.CodeCampaignValidationAccepted)
	}
	if input.ExpectedRevision != aggregate.Revision {
		return reject(DecisionConflict, aggregate, reason.CodeCampaignValidationInvalid)
	}
	transition, ok := TransitionFor(aggregate.State, input.Command)
	if !ok {
		return reject(DecisionInvalidInput, aggregate, reason.CodeCampaignValidationInvalid)
	}
	if transition.Epoch == EpochIncrement {
		if input.ExpectedEpoch != aggregate.CommandEpoch {
			return reject(DecisionConflict, aggregate, reason.CodeCampaignPauseEpochMismatch)
		}
	}
	if transition.Class == TransitionRejected {
		return reject(DecisionUnsafeCondition, aggregate, transition.Reason)
	}
	if transition.Class == TransitionNoOp {
		return reject(DecisionNoOp, aggregate, transition.Reason)
	}
	if !input.GuardsSatisfied {
		return reject(DecisionUnsafeCondition, aggregate, transition.Reason)
	}
	next := aggregate
	if transition.Resolution == StateResolutionRestoreSuspended {
		if !isSuspensible(aggregate.SuspendedPhase) {
			return reject(DecisionInvalidInput, aggregate, reason.CodeInternalInvariantInvalidAggregate)
		}
		next.State = aggregate.SuspendedPhase
		next.SuspendedPhase = ""
	} else {
		next.State = transition.Result
	}
	if transition.Suspended == SuspendedPhaseCapture {
		next.SuspendedPhase = aggregate.State
	}
	if transition.Revision == RevisionIncrement {
		next.Revision++
	}
	if transition.Epoch == EpochIncrement {
		next.CommandEpoch++
	}
	return Decision{Class: DecisionAllowed, Aggregate: next, Reason: transition.Reason, Events: eventSlice(transition.Event), NextIntent: transition.Event}
}
func reject(class DecisionClass, aggregate Aggregate, code reason.Code) Decision {
	return Decision{Class: class, Aggregate: aggregate, Reason: code}
}
func eventSlice(event string) []string {
	if event == "" {
		return nil
	}
	return []string{event}
}
func knownState(state State) bool       { _, err := ParseState(state.String()); return err == nil }
func knownCommand(command Command) bool { _, err := ParseCommand(command.String()); return err == nil }
func isSuspensible(state State) bool {
	return state == StateRunningCanary || state == StateEvaluatingCanary || state == StateRunningWaves
}
