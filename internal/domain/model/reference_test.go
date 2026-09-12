package model

import (
	"testing"

	"pgregory.net/rapid"
)

func TestReferenceModelRejectsImpossibleState(t *testing.T) {
	impossible := CampaignAggregate{State: "ALIEN", Revision: 4}
	got, advanced := Step(impossible, CampaignComplete)
	if advanced || got != impossible {
		t.Fatalf("impossible state changed: %#v, %t", got, advanced)
	}
}

func TestReferenceModelWouldDetectTransitionMutation(t *testing.T) {
	got, advanced := Step(CampaignAggregate{State: CampaignRunningWaves}, CampaignComplete)
	if !advanced || got.State != CampaignCompleted {
		t.Fatalf("reference complete = %#v, %t", got, advanced)
	}
	if got.State == CampaignFailed {
		t.Fatal("deliberate mutated expected state unexpectedly matched")
	}
}

func TestReferenceModelPreservesMonotonicRevisions(t *testing.T) {
	commands := []CampaignCommand{CampaignBeginValidation, CampaignValidationPassed, CampaignValidationFailed, CampaignStartCanary, CampaignCanaryTerminal, CampaignApproveCanary, CampaignReleaseNextWave, CampaignComplete, CampaignPause, CampaignResume, CampaignCancel, CampaignAbort, CampaignAbortFailed, CampaignAbortCancelled}
	rapid.Check(t, func(t *rapid.T) {
		current := CampaignAggregate{State: CampaignDraft}
		for range 100 {
			command := rapid.SampledFrom(commands).Draw(t, "command")
			next, advanced := Step(current, command)
			if advanced && next.Revision != current.Revision+1 {
				t.Fatalf("advanced revision %d -> %d", current.Revision, next.Revision)
			}
			if !advanced && next != current {
				t.Fatalf("rejected command mutated aggregate: %#v -> %#v", current, next)
			}
			current = next
		}
	})
}
