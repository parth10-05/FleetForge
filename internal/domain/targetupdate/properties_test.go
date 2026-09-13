package targetupdate

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/parth10-05/fleetforge/internal/domain/model"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestTargetReducerAgreesWithIndependentSequentialModel(t *testing.T) {
	id := targetCommandID(t, "00000000-0000-4000-8000-0000000000b1")
	rapid.Check(t, func(t *rapid.T) {
		actual := Aggregate{State: StatePending}
		reference := model.TargetAggregate{State: model.TargetPending}
		for range 100 {
			command := rapid.SampledFrom(AllCommands()).Draw(t, "command")
			decision := Reduce(actual, authorizedTargetInput(id, command, actual.Revision, actual.CommandEpoch))
			nextReference, advanced, submit := model.StepTarget(reference, model.TargetCommand(command.String()))
			if advanced {
				if decision.Class != DecisionAllowed || decision.SubmitIntent != submit {
					t.Fatalf("%s/%s result = %#v", actual.State, command, decision)
				}
				if decision.Aggregate.State.String() != string(nextReference.State) || decision.Aggregate.Revision.Uint64() != nextReference.Revision || decision.Aggregate.ResumeStep.String() != string(nextReference.ResumeStep) {
					t.Fatalf("reducer %#v, reference %#v", decision.Aggregate, nextReference)
				}
			} else if decision.Class == DecisionAllowed || decision.Aggregate != actual || decision.SubmitIntent {
				t.Fatalf("rejected command changed aggregate: %#v", decision)
			}
			actual, reference = decision.Aggregate, nextReference
		}
	})
}

func TestTargetObservationCannotCreateSubmissionIntent(t *testing.T) {
	id := targetCommandID(t, "00000000-0000-4000-8000-0000000000b2")
	states := []State{StateUpdateAccepted, StateWaitingReboot, StateVerifying, StateNeedsAttention}
	for _, state := range states {
		decision := Reduce(Aggregate{State: state}, authorizedTargetInput(id, CommandContinueObservation, 0, 0))
		if decision.SubmitIntent {
			t.Fatalf("observation in %s created submission intent", state)
		}
	}
}

func targetCommandID(t testing.TB, raw string) value.CommandID {
	t.Helper()
	id, err := value.ParseCommandID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
