package domainmodel

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/targetupdate"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestCampaignReducerAgreesWithIndependentSequentialModel(t *testing.T) {
	commandID := domainCommandID(t, "00000000-0000-4000-8000-0000000000a1")
	rapid.Check(t, func(t *rapid.T) {
		actual := campaign.Aggregate{State: campaign.StateDraft}
		reference := CampaignAggregate{State: CampaignDraft}
		for range 100 {
			command := rapid.SampledFrom(campaign.AllCommands()).Draw(t, "command")
			decision := campaign.Reduce(actual, campaign.CommandInput{ID: commandID, Command: command, ExpectedRevision: actual.Revision, ExpectedEpoch: actual.CommandEpoch, Authorization: authorizedCampaign()})
			nextReference, advanced := stepCampaign(reference, CampaignCommand(command.String()))
			if advanced {
				if decision.Class != campaign.DecisionAllowed {
					t.Fatalf("%s/%s rejected by reducer: %#v", actual.State, command, decision)
				}
				assertCampaignMatchesReference(t, decision.Aggregate, nextReference)
			} else if decision.Class == campaign.DecisionAllowed || decision.Aggregate != actual {
				t.Fatalf("non-advancing command changed aggregate: %#v -> %#v", actual, decision.Aggregate)
			}
			actual, reference = decision.Aggregate, nextReference
		}
	})
}

func TestCampaignEpochOnlyChangesForPauseOrAbort(t *testing.T) {
	commandID := domainCommandID(t, "00000000-0000-4000-8000-0000000000a2")
	rapid.Check(t, func(t *rapid.T) {
		current := campaign.Aggregate{State: campaign.StateDraft}
		for range 100 {
			command := rapid.SampledFrom(campaign.AllCommands()).Draw(t, "command")
			decision := campaign.Reduce(current, campaign.CommandInput{ID: commandID, Command: command, ExpectedRevision: current.Revision, ExpectedEpoch: current.CommandEpoch, Authorization: authorizedCampaign()})
			if decision.Aggregate.CommandEpoch < current.CommandEpoch || decision.Aggregate.CommandEpoch > current.CommandEpoch+1 {
				t.Fatalf("epoch changed unsafely: %#v -> %#v", current, decision.Aggregate)
			}
			if decision.Aggregate.CommandEpoch == current.CommandEpoch+1 && command != campaign.CommandPause && command != campaign.CommandAbort {
				t.Fatalf("unexpected epoch increment for %s", command)
			}
			current = decision.Aggregate
		}
	})
}

func TestTargetReducerAgreesWithIndependentSequentialModel(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-0000000000b1")
	rapid.Check(t, func(t *rapid.T) {
		actual := targetupdate.Aggregate{State: targetupdate.StatePending}
		reference := TargetAggregate{State: TargetPending}
		for range 100 {
			command := rapid.SampledFrom(targetupdate.AllCommands()).Draw(t, "command")
			decision := targetupdate.Reduce(actual, authorizedTargetInput(id, command, actual.Revision, actual.CommandEpoch))
			nextReference, advanced, submit := stepTarget(reference, TargetCommand(command.String()))
			if advanced {
				if decision.Class != targetupdate.DecisionAllowed || decision.SubmitIntent != submit {
					t.Fatalf("%s/%s result = %#v", actual.State, command, decision)
				}
				if decision.Aggregate.State.String() != string(nextReference.State) || decision.Aggregate.Revision.Uint64() != nextReference.Revision || decision.Aggregate.ResumeStep.String() != string(nextReference.ResumeStep) {
					t.Fatalf("reducer %#v, reference %#v", decision.Aggregate, nextReference)
				}
			} else if decision.Class == targetupdate.DecisionAllowed || decision.Aggregate != actual || decision.SubmitIntent {
				t.Fatalf("rejected command changed aggregate: %#v", decision)
			}
			actual, reference = decision.Aggregate, nextReference
		}
	})
}

func TestTargetObservationCannotCreateSubmissionIntent(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-0000000000b2")
	for _, state := range []targetupdate.State{targetupdate.StateUpdateAccepted, targetupdate.StateWaitingReboot, targetupdate.StateVerifying, targetupdate.StateNeedsAttention} {
		decision := targetupdate.Reduce(targetupdate.Aggregate{State: state}, authorizedTargetInput(id, targetupdate.CommandContinueObservation, 0, 0))
		if decision.SubmitIntent {
			t.Fatalf("observation in %s created submission intent", state)
		}
	}
}

func authorizedCampaign() policy.CampaignAuthorization {
	return policy.NewCampaignAuthorization(
		policy.CampaignGuardValidSnapshot, policy.CampaignGuardCancellationPermitted, policy.CampaignGuardPreflightPassed, policy.CampaignGuardSnapshotFrozen,
		policy.CampaignGuardCorrectableFailure, policy.CampaignGuardAuthorizedStart, policy.CampaignGuardCanaryTargetsTerminal, policy.CampaignGuardPauseAuthorized,
		policy.CampaignGuardAbortAuthorized, policy.CampaignGuardCanaryEvaluated, policy.CampaignGuardFailureBudgetPassed, policy.CampaignGuardPriorWaveEvaluated,
		policy.CampaignGuardAllTargetsTerminal, policy.CampaignGuardFinalPolicyPassed, policy.CampaignGuardSuspendedPhasePresent,
		policy.CampaignGuardInflightOutcomeBlocked, policy.CampaignGuardRemainingWorkResolved,
	)
}

func authorizedTargetInput(id value.CommandID, command targetupdate.Command, revision value.Revision, epoch value.CommandEpoch) targetupdate.CommandInput {
	return targetupdate.CommandInput{ID: id, Command: command, ExpectedRevision: revision, ExpectedEpoch: epoch, Authorization: policy.ApproveTargetTransition(), Submission: policy.NewPersistedSubmissionAdmission(id, epoch), RetryAbsence: policy.NewIndependentAbsenceAndLateCallerExclusion(), SuccessEvidence: policy.NewVersionAndHealthSuccess()}
}

func assertCampaignMatchesReference(t *rapid.T, actual campaign.Aggregate, reference CampaignAggregate) {
	t.Helper()
	if actual.State.String() != string(reference.State) || actual.Revision.Uint64() != reference.Revision || actual.CommandEpoch.Uint64() != reference.CommandEpoch || actual.SuspendedPhase.String() != string(reference.SuspendedPhase) {
		t.Fatalf("reducer aggregate %#v, reference %#v", actual, reference)
	}
}
func domainCommandID(t testing.TB, raw string) value.CommandID {
	t.Helper()
	id, err := value.ParseCommandID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
