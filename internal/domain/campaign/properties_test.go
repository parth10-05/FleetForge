package campaign

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/parth10-05/fleetforge/internal/domain/model"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestCampaignReducerAgreesWithIndependentSequentialModel(t *testing.T) {
	commandID := mustCommandID(t, "00000000-0000-4000-8000-0000000000a1")
	rapid.Check(t, func(t *rapid.T) {
		actual := Aggregate{State: StateDraft}
		reference := model.CampaignAggregate{State: model.CampaignDraft}
		for range 100 {
			command := rapid.SampledFrom(AllCommands()).Draw(t, "command")
			decision := Reduce(actual, CommandInput{ID: commandID, Command: command, ExpectedRevision: actual.Revision, ExpectedEpoch: actual.CommandEpoch, Authorization: authorizedCampaign()})
			nextReference, advanced := model.Step(reference, model.CampaignCommand(command.String()))
			if advanced {
				if decision.Class != DecisionAllowed {
					t.Fatalf("%s/%s rejected by reducer: %#v", actual.State, command, decision)
				}
				assertMatchesReference(t, decision.Aggregate, nextReference)
			} else {
				if decision.Class == DecisionAllowed {
					t.Fatalf("%s/%s unexpectedly advanced: %#v", actual.State, command, decision)
				}
				if decision.Aggregate != actual {
					t.Fatalf("non-advancing command mutated aggregate: %#v -> %#v", actual, decision.Aggregate)
				}
			}
			actual, reference = decision.Aggregate, nextReference
		}
	})
}

func TestCampaignEpochOnlyChangesForPauseOrAbort(t *testing.T) {
	commandID := mustCommandID(t, "00000000-0000-4000-8000-0000000000a2")
	rapid.Check(t, func(t *rapid.T) {
		current := Aggregate{State: StateDraft}
		for range 100 {
			command := rapid.SampledFrom(AllCommands()).Draw(t, "command")
			decision := Reduce(current, CommandInput{ID: commandID, Command: command, ExpectedRevision: current.Revision, ExpectedEpoch: current.CommandEpoch, Authorization: authorizedCampaign()})
			if decision.Aggregate.CommandEpoch < current.CommandEpoch || decision.Aggregate.CommandEpoch > current.CommandEpoch+1 {
				t.Fatalf("epoch changed unsafely: %#v -> %#v", current, decision.Aggregate)
			}
			if decision.Aggregate.CommandEpoch == current.CommandEpoch+1 && command != CommandPause && command != CommandAbort {
				t.Fatalf("unexpected epoch increment for %s", command)
			}
			current = decision.Aggregate
		}
	})
}

func assertMatchesReference(t *rapid.T, actual Aggregate, reference model.CampaignAggregate) {
	t.Helper()
	if actual.State.String() != string(reference.State) || actual.Revision.Uint64() != reference.Revision || actual.CommandEpoch.Uint64() != reference.CommandEpoch || actual.SuspendedPhase.String() != string(reference.SuspendedPhase) {
		t.Fatalf("reducer aggregate %#v, reference %#v", actual, reference)
	}
}

func mustCommandID(t testing.TB, raw string) value.CommandID {
	t.Helper()
	id, err := value.ParseCommandID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
