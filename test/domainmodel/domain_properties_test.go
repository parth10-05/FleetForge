package domainmodel

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"

	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/event"
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/replay"
	"github.com/parth10-05/fleetforge/internal/domain/targetupdate"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	"github.com/parth10-05/fleetforge/internal/domain/wave"
)

// Each subtest is the executable mapping for one property row. Directed prefixes
// prove the stated edge is reached before Rapid explores additional cases.
func TestDirectedPropertyCoverage(t *testing.T) {
	t.Run("campaign states and revisions", propertyCampaignStates)
	t.Run("duplicate commands", propertyDuplicates)
	t.Run("ordered wave release", propertyWaveOrdering)
	t.Run("stale and duplicate evidence", propertyStaleEvidence)
	t.Run("retry budget boundary", propertyRetryBudget)
	t.Run("pause abort epoch fence", propertyPauseAbortEpoch)
	t.Run("exact failure budget", propertyFailureBudget)
	t.Run("deterministic replay", propertyReplay)
	t.Run("recorded retry step", propertyRetrySteps)
	t.Run("persisted pause resume", propertyPauseResume)
	t.Run("attention exits", propertyAttentionExits)
}

func propertyCampaignStates(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-000000000201")
	current := campaign.Aggregate{State: campaign.StateDraft}
	seen := map[campaign.State]bool{}
	for _, command := range []campaign.Command{campaign.CommandBeginValidation, campaign.CommandValidationPassed, campaign.CommandStartCanary, campaign.CommandCanaryTerminal, campaign.CommandApproveCanary, campaign.CommandReleaseNextWave} {
		current = requireCampaignAllowed(t, current, id, command)
		seen[current.State] = true
	}
	for _, state := range []campaign.State{campaign.StateValidating, campaign.StateReady, campaign.StateRunningCanary, campaign.StateEvaluatingCanary, campaign.StateRunningWaves} {
		if !seen[state] {
			t.Fatalf("directed campaign state %s was not reached", state)
		}
	}
	rapid.Check(t, func(t *rapid.T) {
		actual := campaign.Aggregate{State: campaign.StateDraft}
		for range 40 {
			command := rapid.SampledFrom(campaign.AllCommands()).Draw(t, "command")
			decision := campaign.Reduce(actual, campaign.CommandInput{ID: id, Command: command, ExpectedRevision: actual.Revision, ExpectedEpoch: actual.CommandEpoch, Authorization: authorizedCampaign()})
			if decision.Aggregate.Revision < actual.Revision || !campaign.ValidateAggregate(decision.Aggregate) {
				t.Fatalf("invalid campaign result %#v", decision)
			}
			actual = decision.Aggregate
		}
	})
}

func propertyDuplicates(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-000000000202")
	rapid.Check(t, func(t *rapid.T) {
		state := rapid.SampledFrom([]campaign.State{campaign.StateDraft, campaign.StateValidating, campaign.StateReady, campaign.StateRunningCanary, campaign.StateEvaluatingCanary, campaign.StateRunningWaves, campaign.StateAborting, campaign.StateCompleted, campaign.StateFailed, campaign.StateCancelled}).Draw(t, "state")
		before := campaign.Aggregate{State: state}
		decision := campaign.Reduce(before, campaign.CommandInput{ID: id, Command: rapid.SampledFrom(campaign.AllCommands()).Draw(t, "command"), Duplicate: true, Authorization: authorizedCampaign()})
		if decision.Class != campaign.DecisionNoOp || decision.Aggregate != before || len(decision.Events) != 0 {
			t.Fatalf("duplicate changed campaign %#v", decision)
		}
	})
}

func propertyWaveOrdering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ordinal := rapid.Int64Range(2, 40).Draw(t, "ordinal")
		nextOrdinal, err := value.NewWaveOrdinal(ordinal)
		if err != nil {
			t.Fatal(err)
		}
		previousOrdinal, err := value.NewWaveOrdinal(ordinal - 1)
		if err != nil {
			t.Fatal(err)
		}
		next := wave.Wave{Ordinal: nextOrdinal}
		previous := &wave.Wave{Ordinal: previousOrdinal, Terminal: rapid.Bool().Draw(t, "terminal"), Evaluation: rapid.SampledFrom([]wave.Evaluation{wave.EvaluationPending, wave.EvaluationPassed, wave.EvaluationFailed}).Draw(t, "evaluation")}
		got := wave.CanRelease(next, previous)
		want := previous.Terminal && previous.Evaluation == wave.EvaluationPassed
		if got.Allowed != want {
			t.Fatalf("CanRelease = %#v, want %t", got, want)
		}
	})
}

func propertyStaleEvidence(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-000000000203")
	for _, state := range targetupdate.AllStates() {
		before := targetupdate.Aggregate{State: state}
		if state == targetupdate.StateRetryWait {
			before.ResumeStep = targetupdate.ResumeStepPrecheck
		}
		input := authorizedTargetInput(id, targetupdate.CommandContinueObservation, 0, 0)
		input.EvidenceOlder = true
		decision := targetupdate.Reduce(before, input)
		if decision.Class != targetupdate.DecisionNoOp || decision.Aggregate != before || decision.SubmitIntent {
			t.Fatalf("stale evidence in %s = %#v", state, decision)
		}
	}
}

func propertyRetryBudget(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.Int64Range(1, 20).Draw(t, "limit")
		budget, err := value.NewRetryBudget(limit)
		if err != nil {
			t.Fatal(err)
		}
		for attempts := uint64(0); attempts <= uint64(limit)+1; attempts++ {
			decision := policy.ClassifyRetry(policy.RetryInput{Budget: budget, AttemptsUsed: attempts, Stage: policy.RetryStagePrecheck, Cause: policy.RetryCausePrecheckTransient})
			if decision.RetryAllowed != (attempts < uint64(limit)) {
				t.Fatalf("attempt %d of %d = %#v", attempts, limit, decision)
			}
		}
	})
}

func propertyPauseAbortEpoch(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-000000000204")
	for _, state := range []campaign.State{campaign.StateRunningCanary, campaign.StateEvaluatingCanary, campaign.StateRunningWaves} {
		before := campaign.Aggregate{State: state, Revision: 3, CommandEpoch: 7}
		paused := requireCampaignAllowed(t, before, id, campaign.CommandPause)
		if paused.CommandEpoch != before.CommandEpoch+1 || paused.SuspendedPhase != state {
			t.Fatalf("pause did not fence %s: %#v", state, paused)
		}
		aborted := requireCampaignAllowed(t, paused, id, campaign.CommandAbort)
		if aborted.CommandEpoch != paused.CommandEpoch+1 || aborted.SuspendedPhase != "" {
			t.Fatalf("abort did not fence %s: %#v", state, aborted)
		}
	}
}

func propertyFailureBudget(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		denominator := rapid.Int64Range(1, 10).Draw(t, "denominator")
		numerator := rapid.Int64Range(0, denominator).Draw(t, "numerator")
		evaluated := rapid.Int64Range(1, 100).Draw(t, "evaluated")
		failed := rapid.Int64Range(0, evaluated).Draw(t, "failed")
		budget, err := value.NewFailureBudget(numerator, denominator)
		if err != nil {
			t.Fatal(err)
		}
		got := policy.EvaluateFailureBudget(budget, uint64(failed), uint64(evaluated))
		wantReached := failed > 0 && failed*denominator >= numerator*evaluated
		if !got.Valid || got.ThresholdReached != wantReached || got.AdmissionAllowed == wantReached {
			t.Fatalf("budget %d/%d with %d/%d = %#v", numerator, denominator, failed, evaluated, got)
		}
	})
}

func propertyReplay(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-000000000205")
	rapid.Check(t, func(t *rapid.T) {
		trace := replay.CampaignTrace{Version: event.VersionV1, Initial: campaign.Aggregate{State: campaign.StateDraft}}
		for range 12 {
			trace.Actions = append(trace.Actions, campaign.CommandInput{ID: id, Command: rapid.SampledFrom(campaign.AllCommands()).Draw(t, "command"), ExpectedRevision: 0, Authorization: authorizedCampaign()})
		}
		first, err := replay.ReplayCampaign(trace)
		if err != nil {
			t.Fatal(err)
		}
		second, err := replay.ReplayCampaign(trace)
		if err != nil {
			t.Fatal(err)
		}
		left, err := replay.CanonicalJSON(first)
		if err != nil {
			t.Fatal(err)
		}
		right, err := replay.CanonicalJSON(second)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, right) {
			t.Fatalf("replay changed: %s / %s", left, right)
		}
	})
}

func propertyRetrySteps(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-000000000206")
	cases := []struct {
		resume  targetupdate.ResumeStep
		command targetupdate.Command
		want    targetupdate.State
	}{{targetupdate.ResumeStepPrecheck, targetupdate.CommandRetryDue, targetupdate.StatePrecheck}, {targetupdate.ResumeStepSubmitting, targetupdate.CommandAdmitSubmission, targetupdate.StateSubmitting}, {targetupdate.ResumeStepWaitingReboot, targetupdate.CommandRebootObserved, targetupdate.StateWaitingReboot}, {targetupdate.ResumeStepVerifying, targetupdate.CommandVerificationRetryable, targetupdate.StateVerifying}}
	for _, testCase := range cases {
		before := targetupdate.Aggregate{State: targetupdate.StateRetryWait, ResumeStep: testCase.resume}
		decision := targetupdate.Reduce(before, authorizedTargetInput(id, testCase.command, 0, 0))
		if decision.Class != targetupdate.DecisionAllowed || decision.Aggregate.State != testCase.want {
			t.Fatalf("retry %s/%s = %#v", testCase.resume, testCase.command, decision)
		}
	}
	for _, resume := range []targetupdate.ResumeStep{targetupdate.ResumeStepPrecheck, targetupdate.ResumeStepSubmitting, targetupdate.ResumeStepWaitingReboot, targetupdate.ResumeStepVerifying} {
		for _, command := range []targetupdate.Command{targetupdate.CommandRetryDue, targetupdate.CommandAdmitSubmission, targetupdate.CommandRebootObserved, targetupdate.CommandVerificationRetryable} {
			before := targetupdate.Aggregate{State: targetupdate.StateRetryWait, ResumeStep: resume}
			decision := targetupdate.Reduce(before, authorizedTargetInput(id, command, 0, 0))
			if decision.Class == targetupdate.DecisionAllowed && decision.Aggregate.State == targetupdate.StateSubmitting && resume != targetupdate.ResumeStepSubmitting {
				t.Fatalf("unsafe retry submit from %s", resume)
			}
		}
	}
}

func propertyPauseResume(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-000000000207")
	for _, state := range []campaign.State{campaign.StateRunningCanary, campaign.StateEvaluatingCanary, campaign.StateRunningWaves} {
		before := campaign.Aggregate{State: state, Revision: 2, CommandEpoch: 4}
		paused := requireCampaignAllowed(t, before, id, campaign.CommandPause)
		resumed := requireCampaignAllowed(t, paused, id, campaign.CommandResume)
		if resumed.State != state || resumed.SuspendedPhase != "" {
			t.Fatalf("resume skipped %s: %#v", state, resumed)
		}
	}
}

func propertyAttentionExits(t *testing.T) {
	id := domainCommandID(t, "00000000-0000-4000-8000-000000000208")
	cases := []struct {
		command targetupdate.Command
		state   targetupdate.State
		exit    targetupdate.AttentionExit
	}{{targetupdate.CommandAdoptOperation, targetupdate.StateUpdateAccepted, targetupdate.AttentionExitAdopt}, {targetupdate.CommandAttentionRetry, targetupdate.StateRetryWait, targetupdate.AttentionExitProofRetry}, {targetupdate.CommandAttentionTerminal, targetupdate.StateFailedTerminal, targetupdate.AttentionExitTerminal}, {targetupdate.CommandContinueObservation, targetupdate.StateNeedsAttention, targetupdate.AttentionExitNone}}
	for _, testCase := range cases {
		decision := targetupdate.Reduce(targetupdate.Aggregate{State: targetupdate.StateNeedsAttention}, authorizedTargetInput(id, testCase.command, 0, 0))
		if decision.Class != targetupdate.DecisionAllowed || decision.Aggregate.State != testCase.state || decision.Attention != testCase.exit {
			t.Fatalf("attention %s = %#v", testCase.command, decision)
		}
	}
}

func requireCampaignAllowed(t testing.TB, before campaign.Aggregate, id value.CommandID, command campaign.Command) campaign.Aggregate {
	t.Helper()
	decision := campaign.Reduce(before, campaign.CommandInput{ID: id, Command: command, ExpectedRevision: before.Revision, ExpectedEpoch: before.CommandEpoch, Authorization: authorizedCampaign()})
	if decision.Class != campaign.DecisionAllowed {
		t.Fatalf("%s/%s = %#v", before.State, command, decision)
	}
	return decision.Aggregate
}
func mustWaveOrdinal(t testing.TB, raw int64) value.WaveOrdinal {
	t.Helper()
	ordinal, err := value.NewWaveOrdinal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ordinal
}
