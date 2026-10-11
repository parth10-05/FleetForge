package campaignplan

import (
	"errors"
	"reflect"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
	"pgregory.net/rapid"
)

func TestBuildIsIndependentOfInputOrderAndExplainsAssignments(t *testing.T) {
	candidates := []Candidate{
		candidate(t, "00000000-0000-4000-8000-000000000001", "iad", "a"),
		candidate(t, "00000000-0000-4000-8000-000000000002", "iad", "b"),
		candidate(t, "00000000-0000-4000-8000-000000000003", "sfo", "a"),
		candidate(t, "00000000-0000-4000-8000-000000000004", "sfo", "b"),
	}
	first, err := Build(Input{Policy: validPolicy(Size{Unit: SizeUnitAbsolute, Value: 2}), Seed: "campaign-42", Algorithm: SelectionVersionV1, Candidates: candidates})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(Input{Policy: validPolicy(Size{Unit: SizeUnitAbsolute, Value: 2}), Seed: "campaign-42", Algorithm: SelectionVersionV1, Candidates: []Candidate{candidates[3], candidates[2], candidates[1], candidates[0]}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash != second.Hash || !reflect.DeepEqual(first.Assignments, second.Assignments) || first.EffectiveCanary != 2 || first.EffectiveLater != 2 {
		t.Fatalf("reordered plan changed: %#v / %#v", first, second)
	}
	seenRacks := map[string]struct{}{}
	for _, assignment := range first.Assignments {
		if assignment.Reason == "" || assignment.Ordinal == 0 {
			t.Fatalf("unexplained assignment: %#v", assignment)
		}
		if assignment.Cohort == CohortCanary {
			seenRacks[assignment.Site+"/"+assignment.Rack] = struct{}{}
		}
	}
	if len(seenRacks) != 2 {
		t.Fatalf("canary did not diversify racks: %#v", first.Assignments)
	}
}

func TestPolicyBoundaryValidationAndRounding(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		policy Policy
		count  uint64
		want   uint64
		valid  bool
	}{
		{name: "one target permits complete canary", policy: validPolicy(Size{Unit: SizeUnitPercent, Value: 100}), count: 1, want: 1, valid: true},
		{name: "percentage rounds upward", policy: validPolicy(Size{Unit: SizeUnitPercent, Value: 34}), count: 3, want: 2, valid: true},
		{name: "multi target cannot consume later wave", policy: validPolicy(Size{Unit: SizeUnitPercent, Value: 100}), count: 2, valid: false},
		{name: "strict capacity remains unsupported", policy: func() Policy {
			p := validPolicy(Size{Unit: SizeUnitAbsolute, Value: 1})
			p.StrictWorstCaseCapacity = true
			return p
		}(), count: 2, valid: false},
		{name: "zero failure denominator", policy: func() Policy {
			p := validPolicy(Size{Unit: SizeUnitAbsolute, Value: 1})
			p.FailureBudget.Denominator = 0
			return p
		}(), count: 2, valid: false},
		{name: "limit inversion", policy: func() Policy { p := validPolicy(Size{Unit: SizeUnitAbsolute, Value: 1}); p.Limits.Site = 3; return p }(), count: 2, valid: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.policy.Validate(testCase.count)
			if testCase.valid {
				if err != nil {
					t.Fatal(err)
				}
				got, err := testCase.policy.EffectiveCanary(testCase.count)
				if err != nil || got != testCase.want {
					t.Fatalf("effective canary = %d, %v", got, err)
				}
			} else if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestBuildRejectsUnknownOrMalformedInputs(t *testing.T) {
	good := candidate(t, "00000000-0000-4000-8000-000000000001", "iad", "a")
	for _, input := range []Input{
		{Policy: validPolicy(Size{Unit: SizeUnitAbsolute, Value: 1}), Seed: "seed", Algorithm: "selection/v2", Candidates: []Candidate{good}},
		{Policy: validPolicy(Size{Unit: SizeUnitAbsolute, Value: 1}), Seed: "", Algorithm: SelectionVersionV1, Candidates: []Candidate{good}},
		{Policy: validPolicy(Size{Unit: SizeUnitAbsolute, Value: 1}), Seed: "seed", Algorithm: SelectionVersionV1, Candidates: []Candidate{good, good}},
		{Policy: validPolicy(Size{Unit: SizeUnitAbsolute, Value: 1}), Seed: "seed", Algorithm: SelectionVersionV1, Candidates: []Candidate{{TargetID: good.TargetID, Site: "", Rack: "a", Family: "synthetic-v1"}}},
	} {
		if _, err := Build(input); !errors.Is(err, ErrInvalidPlanInput) {
			t.Fatalf("Build(%#v) error = %v", input, err)
		}
	}
}

func TestBuildAlwaysAssignsEveryGeneratedCandidateOnce(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(1, 40).Draw(t, "count")
		candidates := make([]Candidate, 0, count)
		for index := 0; index < count; index++ {
			id, _ := value.ParseTargetID(uuidFor(index + 1))
			revision, _ := value.NewRevision(1)
			candidates = append(candidates, Candidate{TargetID: id, Revision: revision, Site: "site-" + string(rune('a'+index%3)), Rack: "rack-" + string(rune('a'+index%7)), Family: "synthetic-v1"})
		}
		canary := uint64(1)
		if count > 1 {
			canary = uint64(rapid.IntRange(1, count-1).Draw(t, "canary"))
		}
		plan, err := Build(Input{Policy: validPolicy(Size{Unit: SizeUnitAbsolute, Value: canary}), Seed: "property-seed", Algorithm: SelectionVersionV1, Candidates: candidates})
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]Assignment{}
		for _, assignment := range plan.Assignments {
			if _, exists := seen[assignment.TargetID]; exists {
				t.Fatalf("duplicate %s", assignment.TargetID)
			}
			seen[assignment.TargetID] = assignment
		}
		if len(seen) != count || plan.EffectiveCanary+plan.EffectiveLater != uint64(count) {
			t.Fatalf("incomplete plan: %#v", plan)
		}
	})
}

func validPolicy(canary Size) Policy {
	return Policy{Version: PolicyVersionV1, Canary: canary, Limits: Limits{Global: 1, Site: 1, Rack: 1}, FailureBudget: Fraction{Numerator: 1, Denominator: 4}, ExpectedFirmwareVersion: "v2", RequiredHealthChecks: []string{"boot"}, Maintenance: MaintenanceWindow{Timezone: "UTC", Weekdays: []int{1, 2}, StartMinute: 60, EndMinute: 120}, MaximumDurationSeconds: 3600, RetryBudget: 3, ObservationWindowSecond: 300, ObservationEverySecond: 60, ObservationFreshSecond: 120, GovernanceID: "change-42"}
}
func candidate(t testing.TB, rawID, site, rack string) Candidate {
	t.Helper()
	id, err := value.ParseTargetID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := value.NewRevision(1)
	if err != nil {
		t.Fatal(err)
	}
	return Candidate{TargetID: id, Revision: revision, Site: site, Rack: rack, Family: "synthetic-v1"}
}
func uuidFor(index int) string { return "00000000-0000-4000-8000-" + leftPad(index) }
func leftPad(value int) string {
	raw := ""
	for value > 0 {
		raw = string(rune('0'+value%10)) + raw
		value /= 10
	}
	for len(raw) < 12 {
		raw = "0" + raw
	}
	return raw
}
