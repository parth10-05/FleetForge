package replay

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/event"
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/targetupdate"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestReplayCampaignIsByteStableAndMatchesGoldenTrace(t *testing.T) {
	trace := campaignTrace(t)
	first, err := ReplayCampaign(trace)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReplayCampaign(trace)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := CanonicalJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := CanonicalJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("replay was not byte-stable: %s / %s", firstJSON, secondJSON)
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	want, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "test", "fixtures", "domain", "v1", "campaign-replay.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, bytes.TrimSpace(want)) {
		t.Fatalf("replay = %s, want %s", firstJSON, want)
	}
}

func TestReplayFailsClosedForUnknownVersionAndOrderIsExplicit(t *testing.T) {
	trace := campaignTrace(t)
	trace.Version = "v2"
	if _, err := ReplayCampaign(trace); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("unknown version error = %v", err)
	}
	trace = campaignTrace(t)
	trace.Actions[0], trace.Actions[1] = trace.Actions[1], trace.Actions[0]
	got, err := ReplayCampaign(trace)
	if err != nil {
		t.Fatal(err)
	}
	if got.Aggregate.State != campaign.StateValidating.String() || got.Aggregate.Revision != 1 || got.Decisions[0].Class != string(campaign.DecisionConflict) {
		t.Fatalf("reordered trace was not rejected deterministically: %#v", got)
	}
}

func TestTargetReplayIsByteStable(t *testing.T) {
	id := commandID(t, "00000000-0000-4000-8000-0000000000b3")
	trace := TargetUpdateTrace{Version: event.VersionV1, Initial: targetupdate.Aggregate{State: targetupdate.StatePending}, Actions: []targetupdate.CommandInput{
		authorizedTargetAction(id, targetupdate.CommandStartPrecheck, 0, 0),
		authorizedTargetAction(id, targetupdate.CommandPrecheckPassed, 1, 0),
		authorizedTargetAction(id, targetupdate.CommandAdmitSubmission, 2, 0),
	}}
	first, err := ReplayTargetUpdate(trace)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReplayTargetUpdate(trace)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := CanonicalJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := CanonicalJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("target replay varied: %s / %s", firstJSON, secondJSON)
	}
}

func campaignTrace(t *testing.T) CampaignTrace {
	t.Helper()
	first := commandID(t, "00000000-0000-4000-8000-000000000011")
	second := commandID(t, "00000000-0000-4000-8000-000000000012")
	third := commandID(t, "00000000-0000-4000-8000-000000000013")
	return CampaignTrace{Version: event.VersionV1, Initial: campaign.Aggregate{State: campaign.StateDraft}, Actions: []campaign.CommandInput{
		{ID: first, Command: campaign.CommandBeginValidation, Authorization: authorizedCampaignAction()},
		{ID: second, Command: campaign.CommandValidationPassed, ExpectedRevision: 1, Authorization: authorizedCampaignAction()},
		{ID: third, Command: campaign.CommandStartCanary, ExpectedRevision: 2, Authorization: authorizedCampaignAction()},
	}}
}

func authorizedTargetAction(id value.CommandID, command targetupdate.Command, revision value.Revision, epoch value.CommandEpoch) targetupdate.CommandInput {
	return targetupdate.CommandInput{
		ID: id, Command: command, ExpectedRevision: revision, ExpectedEpoch: epoch,
		Authorization:   policy.ApproveTargetTransition(),
		Submission:      policy.NewPersistedSubmissionAdmission(id, epoch),
		RetryAbsence:    policy.NewIndependentAbsenceAndLateCallerExclusion(),
		SuccessEvidence: policy.NewVersionAndHealthSuccess(),
	}
}

func authorizedCampaignAction() policy.CampaignAuthorization {
	return policy.NewCampaignAuthorization(
		policy.CampaignGuardValidSnapshot, policy.CampaignGuardCancellationPermitted,
		policy.CampaignGuardPreflightPassed, policy.CampaignGuardSnapshotFrozen,
		policy.CampaignGuardCorrectableFailure, policy.CampaignGuardAuthorizedStart,
		policy.CampaignGuardCanaryTargetsTerminal, policy.CampaignGuardPauseAuthorized,
		policy.CampaignGuardAbortAuthorized, policy.CampaignGuardCanaryEvaluated,
		policy.CampaignGuardFailureBudgetPassed, policy.CampaignGuardPriorWaveEvaluated,
		policy.CampaignGuardAllTargetsTerminal, policy.CampaignGuardFinalPolicyPassed,
		policy.CampaignGuardSuspendedPhasePresent, policy.CampaignGuardInflightOutcomeBlocked,
		policy.CampaignGuardRemainingWorkResolved,
	)
}

func commandID(t *testing.T, raw string) value.CommandID {
	t.Helper()
	id, err := value.ParseCommandID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
