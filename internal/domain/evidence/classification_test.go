package evidence

import (
	"errors"
	"strings"
	"testing"
)

func TestAmbiguityClassificationIsConservative(t *testing.T) {
	testCases := []struct {
		name    string
		signals AmbiguitySignals
		want    AmbiguityOutcome
	}{
		{name: "matching operation", signals: AmbiguitySignals{MatchingOperationObserved: true}, want: AmbiguityMatchingOperationAdopted},
		{name: "expected version", signals: AmbiguitySignals{ExpectedVersionObserved: true}, want: AmbiguityExpectedVersionObserved},
		{name: "independent absence proof", signals: AmbiguitySignals{AbsenceProof: AbsenceProof{IndependentJournalConfirmed: true, LateCallerExcluded: true}}, want: AmbiguityProvenAbsent},
		{name: "contradiction wins", signals: AmbiguitySignals{ContradictoryEvidence: true, ExpectedVersionObserved: true}, want: AmbiguityContradicted},
		{name: "task absent is inconclusive", signals: AmbiguitySignals{TaskAbsent: true}, want: AmbiguityInconclusive},
		{name: "version absent is inconclusive", signals: AmbiguitySignals{VersionAbsent: true}, want: AmbiguityInconclusive},
		{name: "proof without late caller exclusion is inconclusive", signals: AmbiguitySignals{AbsenceProof: AbsenceProof{IndependentJournalConfirmed: true}}, want: AmbiguityInconclusive},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ClassifyAmbiguity(testCase.signals); got != testCase.want {
				t.Fatalf("ClassifyAmbiguity(%+v) = %q, want %q", testCase.signals, got, testCase.want)
			}
		})
	}
}

func TestAmbiguityOutcomeParserIsStrict(t *testing.T) {
	for _, outcome := range AllAmbiguityOutcomes() {
		got, err := ParseAmbiguityOutcome(outcome.String())
		if err != nil || got != outcome {
			t.Fatalf("ParseAmbiguityOutcome(%q) = %q, %v", outcome, got, err)
		}
	}
	for _, raw := range []string{"", "INCONCLUSIVE_RETRY", strings.ToLower(string(AmbiguityInconclusive))} {
		_, err := ParseAmbiguityOutcome(raw)
		if !errors.Is(err, ErrInvalidAmbiguityOutcome) {
			t.Fatalf("ParseAmbiguityOutcome(%q) error = %v, want ErrInvalidAmbiguityOutcome", raw, err)
		}
	}
	outcomes := AllAmbiguityOutcomes()
	outcomes[0] = "MUTATED"
	if AllAmbiguityOutcomes()[0] != AmbiguityMatchingOperationAdopted {
		t.Fatal("AllAmbiguityOutcomes exposed mutable registry storage")
	}
}
