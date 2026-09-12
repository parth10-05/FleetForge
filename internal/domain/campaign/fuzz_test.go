package campaign

import (
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func FuzzCampaignReducerRejectsMalformedInputsWithoutMutation(f *testing.F) {
	f.Add("UNKNOWN", "UNKNOWN")
	f.Add("DRAFT", "BEGIN_VALIDATION")
	f.Fuzz(func(t *testing.T, rawState, rawCommand string) {
		id, _ := value.ParseCommandID("00000000-0000-4000-8000-0000000000c1")
		before := Aggregate{State: State(rawState)}
		decision := Reduce(before, CommandInput{ID: id, Command: Command(rawCommand), GuardsSatisfied: true})
		if decision.Class == DecisionAllowed && decision.Aggregate.Revision != before.Revision+1 {
			t.Fatalf("allowed malformed replay did not increment once")
		}
		if decision.Class != DecisionAllowed && decision.Aggregate != before {
			t.Fatalf("rejected malformed input mutated aggregate")
		}
	})
}
