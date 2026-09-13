package targetupdate

import (
	"github.com/parth10-05/fleetforge/internal/domain/reason"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

type Aggregate struct {
	State        State
	Revision     value.Revision
	CommandEpoch value.CommandEpoch
	ResumeStep   ResumeStep
}
type CommandInput struct {
	ID               value.CommandID
	Command          Command
	ExpectedRevision value.Revision
	ExpectedEpoch    value.CommandEpoch
	Duplicate        bool
	GuardsSatisfied  bool
	EvidenceOlder    bool
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
	Class        DecisionClass
	Aggregate    Aggregate
	Reason       reason.Code
	SubmitIntent bool
	Attention    AttentionExit
}

func Reduce(aggregate Aggregate, input CommandInput) Decision {
	if input.ID.IsZero() || !ValidateAggregate(aggregate) || !knownCommand(input.Command) {
		return reject(DecisionInvalidInput, aggregate, reason.CodeInternalInvariantInvalidAggregate)
	}
	if input.Duplicate || input.EvidenceOlder {
		return reject(DecisionNoOp, aggregate, reason.CodeTargetAmbiguityInconclusive)
	}
	if input.ExpectedRevision != aggregate.Revision {
		return reject(DecisionConflict, aggregate, reason.CodeTargetPrecheckIneligible)
	}
	transition, ok := TransitionFor(aggregate.State, input.Command)
	if !ok {
		return reject(DecisionInvalidInput, aggregate, reason.CodeTargetPrecheckIneligible)
	}
	if transition.SubmitIntent && input.ExpectedEpoch != aggregate.CommandEpoch {
		return reject(DecisionConflict, aggregate, reason.CodeTargetSubmissionNotAuthorized)
	}
	if aggregate.State == StateRetryWait && transition.Class == TransitionAllowed && transition.Resume != aggregate.ResumeStep {
		return reject(DecisionUnsafeCondition, aggregate, reason.CodeTargetPrecheckIneligible)
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
	next.State = transition.Result
	next.Revision++
	if transition.Result == StateRetryWait {
		next.ResumeStep = transition.Resume
	} else if aggregate.State == StateRetryWait {
		next.ResumeStep = ""
	}
	return Decision{Class: DecisionAllowed, Aggregate: next, Reason: transition.Reason, SubmitIntent: transition.SubmitIntent, Attention: transition.Attention}
}
func reject(class DecisionClass, aggregate Aggregate, code reason.Code) Decision {
	return Decision{Class: class, Aggregate: aggregate, Reason: code}
}
func knownState(state State) bool       { _, err := ParseState(state.String()); return err == nil }
func knownCommand(command Command) bool { _, err := ParseCommand(command.String()); return err == nil }
