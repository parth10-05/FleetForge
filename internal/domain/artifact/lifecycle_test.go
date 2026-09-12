package artifact

import "testing"

func TestArtifactLifecycleAndReadinessAreSeparate(t *testing.T) {
	if !CanTransition(StateVerifying, StateVerified) || !CanTransition(StateVerifying, StateRejected) {
		t.Fatal("verification terminal outcomes must be explicit")
	}
	if CanTransition(StateRejected, StateVerifying) || CanTransition(StateRetired, StateVerified) {
		t.Fatal("terminal artifact states must not re-enter verification")
	}
	if !StateVerified.IsReadyForNewCampaign() || StateUploaded.IsReadyForNewCampaign() || StateRetired.IsReadyForNewCampaign() {
		t.Fatal("artifact readiness must require verified state")
	}
	for _, state := range []State{StateRejected, StateRetired, StateExpired} {
		if !state.IsTerminal() {
			t.Fatalf("%s must be terminal", state)
		}
	}
}
