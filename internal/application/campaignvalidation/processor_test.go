package campaignvalidation

import (
	"context"
	"strings"
	"testing"
	"time"

	apppreflight "github.com/parth10-05/fleetforge/internal/application/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	domainpreflight "github.com/parth10-05/fleetforge/internal/domain/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestValidationProcessorPreparesStableFinalization(t *testing.T) {
	result, artifactID := validationSnapshot(t)
	loader := validationInputLoader{input: ValidationInput{ArtifactID: artifactID.String(), PolicyVersionID: "33333333-3333-4333-8333-333333333333", Selector: result.Snapshot.Selector(), Seed: "campaign-seed", Algorithm: campaignplan.SelectionVersionV1, Maximum: 10, Policy: validationPolicy()}}
	finalizer := &recordingFinalizer{}
	processor := NewValidationProcessor(loader, validationEvaluator{result: result}, finalizer, func() time.Time { return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) })
	work := Work{ID: "11111111-1111-4111-8111-111111111111", CommandID: "22222222-2222-4222-8222-222222222222", CampaignID: "33333333-3333-4333-8333-333333333334", LeaseToken: "44444444-4444-4444-8444-444444444444", ExpectedRevision: 1}
	if err := processor.ValidateCampaign(context.Background(), work); err != nil {
		t.Fatal(err)
	}
	first := finalizer.request
	if err := processor.ValidateCampaign(context.Background(), work); err != nil {
		t.Fatal(err)
	}
	second := finalizer.request
	if first.CandidateID == "" || first.CandidateID != second.CandidateID || first.CanaryWaveID != second.CanaryWaveID || first.LaterWaveID != second.LaterWaveID || len(first.Members) != 2 || len(second.Members) != 2 {
		t.Fatalf("unstable finalization first=%+v second=%+v", first, second)
	}
	for index := range first.Members {
		if first.Members[index] != second.Members[index] {
			t.Fatalf("member[%d] changed: first=%+v second=%+v", index, first.Members[index], second.Members[index])
		}
	}
}

func TestValidationProcessorFinalizesBlockedEvaluationAsRejection(t *testing.T) {
	result, artifactID := validationSnapshot(t)
	loader := validationInputLoader{input: ValidationInput{ArtifactID: artifactID.String(), PolicyVersionID: "33333333-3333-4333-8333-333333333333", Selector: result.Snapshot.Selector(), Seed: "campaign-seed", Algorithm: campaignplan.SelectionVersionV1, Maximum: 10, Policy: validationPolicy()}}
	finalizer := &recordingFinalizer{}
	processor := NewValidationProcessor(loader, validationEvaluator{err: blockedEvaluation{}}, finalizer, func() time.Time { return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) })
	work := Work{ID: "11111111-1111-4111-8111-111111111111", CommandID: "22222222-2222-4222-8222-222222222222", CampaignID: "33333333-3333-4333-8333-333333333334", LeaseToken: "44444444-4444-4444-8444-444444444444", ExpectedRevision: 1}
	if err := processor.ValidateCampaign(context.Background(), work); err != nil {
		t.Fatal(err)
	}
	if finalizer.rejection.ReasonCode != "campaign.validation.blocked" || finalizer.rejection.Work.ID != work.ID {
		t.Fatalf("rejection=%+v", finalizer.rejection)
	}
}

type validationInputLoader struct{ input ValidationInput }

func (loader validationInputLoader) LoadCampaignValidationInput(context.Context, Work) (ValidationInput, error) {
	return loader.input, nil
}

type validationEvaluator struct {
	result apppreflight.Result
	err    error
}

func (evaluator validationEvaluator) EvaluateSnapshot(context.Context, apppreflight.Request) (apppreflight.Result, error) {
	return evaluator.result, evaluator.err
}

type recordingFinalizer struct {
	request   FinalizationRequest
	rejection RejectionRequest
}

func (finalizer *recordingFinalizer) FinalizeCampaignValidation(_ context.Context, request FinalizationRequest) error {
	finalizer.request = request
	return nil
}
func (finalizer *recordingFinalizer) RejectCampaignValidation(_ context.Context, request RejectionRequest) error {
	finalizer.rejection = request
	return nil
}

type blockedEvaluation struct{}

func (blockedEvaluation) Error() string { return "blocked" }
func (blockedEvaluation) Unwrap() error { return apppreflight.ErrBlocked }

func validationSnapshot(t *testing.T) (apppreflight.Result, value.ArtifactID) {
	t.Helper()
	artifactID, _ := value.ParseArtifactID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	digest, _ := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "firmware.bin", "version-1", 1, digest)
	if err != nil {
		t.Fatal(err)
	}
	selector, err := inventory.ParseSelector([]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	targets := make([]domainpreflight.Target, 0, 2)
	for _, rawID := range []string{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "cccccccc-cccc-4ccc-8ccc-cccccccccccc"} {
		id, _ := value.ParseTargetID(rawID)
		labels, err := value.NewFailureDomainLabels(map[string]string{"site": "west", "rack": rawID[:1], "family": "synthetic"})
		if err != nil {
			t.Fatal(err)
		}
		family, _ := value.ParseLabelValue("synthetic")
		compatibility, err := artifact.EvaluateCompatibility(artifact.CompatibilityPolicyV1, artifact.StateVerified, family, family)
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, domainpreflight.Target{ID: id, Revision: 1, Labels: labels, Family: family, Compatibility: compatibility})
	}
	snapshot, err := domainpreflight.NewSnapshot(selector, artifactID, object, targets)
	if err != nil {
		t.Fatal(err)
	}
	return apppreflight.Result{Snapshot: snapshot, Report: []byte(`{"state":"passed","target_count":2}`)}, artifactID
}

func validationPolicy() campaignplan.Policy {
	return campaignplan.Policy{Version: campaignplan.PolicyVersionV1, Canary: campaignplan.Size{Unit: campaignplan.SizeUnitAbsolute, Value: 1}, Limits: campaignplan.Limits{Global: 2, Site: 2, Rack: 1}, FailureBudget: campaignplan.Fraction{Numerator: 1, Denominator: 2}, ExpectedFirmwareVersion: "v2", RequiredHealthChecks: []string{"boot"}, Maintenance: campaignplan.MaintenanceWindow{Timezone: "UTC", Weekdays: []int{1}, StartMinute: 1, EndMinute: 2}, MaximumDurationSeconds: 60, RetryBudget: 1, ObservationWindowSecond: 30, ObservationEverySecond: 10, ObservationFreshSecond: 10, GovernanceID: "change-1"}
}
