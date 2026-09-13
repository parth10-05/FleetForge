package targetupdate

import "testing"

func TestCatalogIsCartesianAndRetryAttentionSafe(t *testing.T) {
	transitions := AllTransitions()
	if len(transitions) != len(AllStates())*len(AllCommands()) {
		t.Fatalf("transition count = %d", len(transitions))
	}
	seen := map[State]map[Command]bool{}
	for _, transition := range transitions {
		if seen[transition.From] == nil {
			seen[transition.From] = map[Command]bool{}
		}
		if seen[transition.From][transition.Command] {
			t.Fatalf("duplicate %s/%s", transition.From, transition.Command)
		}
		seen[transition.From][transition.Command] = true
		if transition.Class == TransitionAllowed && transition.Result == StateRetryWait && transition.Resume == "" {
			t.Fatalf("retry %s/%s lacks resume step", transition.From, transition.Command)
		}
		if transition.From == StateWaitingReboot || transition.From == StateVerifying {
			if transition.Result == StateRetryWait && transition.Resume == ResumeStepSubmitting {
				t.Fatalf("observation retry %s/%s points to submit", transition.From, transition.Command)
			}
		}
	}
	attention := []AttentionExit{}
	for _, command := range AllCommands() {
		transition, _ := TransitionFor(StateNeedsAttention, command)
		if transition.Class == TransitionAllowed && transition.Attention != AttentionExitNone {
			attention = append(attention, transition.Attention)
		}
	}
	if len(attention) != 3 || attention[0] != AttentionExitAdopt || attention[1] != AttentionExitProofRetry || attention[2] != AttentionExitTerminal {
		t.Fatalf("attention exits = %v", attention)
	}
}

func TestSubmissionIntentOnlyOccursOnApprovedPaths(t *testing.T) {
	for _, transition := range AllTransitions() {
		if transition.SubmitIntent && !(transition.From == StateSubmitting && transition.Command == CommandAdmitSubmission || transition.From == StateRetryWait && transition.Command == CommandAdmitSubmission) {
			t.Fatalf("unexpected submission intent %s/%s", transition.From, transition.Command)
		}
	}
}
