package targetupdate

import (
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func FuzzTargetReducerRejectsMalformedInputsWithoutSubmission(f *testing.F) {
	f.Add("UNKNOWN", "UNKNOWN")
	f.Add("PENDING", "START_PRECHECK")
	f.Fuzz(func(t *testing.T, rawState, rawCommand string) {
		id, _ := value.ParseCommandID("00000000-0000-4000-8000-0000000000c2")
		before := Aggregate{State: State(rawState)}
		decision := Reduce(before, CommandInput{ID: id, Command: Command(rawCommand), GuardsSatisfied: true})
		if decision.Class != DecisionAllowed && (decision.Aggregate != before || decision.SubmitIntent) {
			t.Fatalf("rejected malformed input created mutation or submit intent")
		}
	})
}
