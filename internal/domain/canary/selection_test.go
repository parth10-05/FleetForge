package canary

import (
	"reflect"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestSelectIsIndependentOfCandidateOrderAndRecordsProvenance(t *testing.T) {
	all := []Candidate{candidate(t, "00000000-0000-4000-8000-000000000001", "east", "a"), candidate(t, "00000000-0000-4000-8000-000000000002", "east", "b"), candidate(t, "00000000-0000-4000-8000-000000000003", "west", "a")}
	forward := Select(SelectionInput{Algorithm: AlgorithmSHA256V1, Seed: "campaign-42", Count: 2, Candidates: all})
	reversed := Select(SelectionInput{Algorithm: AlgorithmSHA256V1, Seed: "campaign-42", Count: 2, Candidates: []Candidate{all[2], all[1], all[0]}})
	if !forward.Valid || !reversed.Valid {
		t.Fatalf("valid selections = %#v / %#v", forward, reversed)
	}
	if !reflect.DeepEqual(forward.Selected, reversed.Selected) || forward.Provenance != reversed.Provenance {
		t.Fatalf("order changed result: %#v / %#v", forward, reversed)
	}
	if forward.Algorithm != AlgorithmSHA256V1 || forward.Provenance == "" {
		t.Fatalf("missing versioned provenance: %#v", forward)
	}
}

func TestSelectRejectsDuplicateInsufficientAndUnknownContracts(t *testing.T) {
	first := candidate(t, "00000000-0000-4000-8000-000000000001", "east", "a")
	second := candidate(t, "00000000-0000-4000-8000-000000000002", "west", "b")
	for _, input := range []SelectionInput{
		{Algorithm: AlgorithmSHA256V1, Seed: "seed", Count: 2, Candidates: []Candidate{first}},
		{Algorithm: AlgorithmSHA256V1, Seed: "seed", Count: 1, Candidates: []Candidate{first, first}},
		{Algorithm: "future-v2", Seed: "seed", Count: 1, Candidates: []Candidate{first, second}},
	} {
		if got := Select(input); got.Valid || len(got.Selected) != 0 {
			t.Fatalf("invalid selection = %#v", got)
		}
	}
}

func candidate(t *testing.T, rawID, site, rack string) Candidate {
	t.Helper()
	id, err := value.ParseTargetID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := value.NewFailureDomainLabels(map[string]string{"site": site, "rack": rack})
	if err != nil {
		t.Fatal(err)
	}
	return Candidate{TargetID: id, Domains: labels}
}
