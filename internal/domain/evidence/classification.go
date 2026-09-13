package evidence

// AmbiguityOutcome is the sole result class used to interpret uncertain
// submission evidence. It is intentionally conservative.
type AmbiguityOutcome string

const (
	AmbiguityMatchingOperationAdopted AmbiguityOutcome = "MATCHING_OPERATION_ADOPTED"
	AmbiguityExpectedVersionObserved  AmbiguityOutcome = "EXPECTED_VERSION_OBSERVED"
	AmbiguityProvenAbsent             AmbiguityOutcome = "PROVEN_ABSENT"
	AmbiguityContradicted             AmbiguityOutcome = "CONTRADICTED"
	AmbiguityInconclusive             AmbiguityOutcome = "INCONCLUSIVE"
)

var allAmbiguityOutcomes = []AmbiguityOutcome{
	AmbiguityMatchingOperationAdopted,
	AmbiguityExpectedVersionObserved,
	AmbiguityProvenAbsent,
	AmbiguityContradicted,
	AmbiguityInconclusive,
}

func ParseAmbiguityOutcome(raw string) (AmbiguityOutcome, error) {
	outcome := AmbiguityOutcome(raw)
	for _, candidate := range allAmbiguityOutcomes {
		if outcome == candidate {
			return outcome, nil
		}
	}
	return "", ErrInvalidAmbiguityOutcome
}

func (outcome AmbiguityOutcome) String() string { return string(outcome) }

func AllAmbiguityOutcomes() []AmbiguityOutcome {
	return append([]AmbiguityOutcome(nil), allAmbiguityOutcomes...)
}

// AbsenceProof records the two independent conditions required before an
// ambiguous submission can safely be classified as absent.
type AbsenceProof struct {
	IndependentJournalConfirmed bool
	LateCallerExcluded          bool
}

func (proof AbsenceProof) IsSufficient() bool {
	return proof.IndependentJournalConfirmed && proof.LateCallerExcluded
}

// AmbiguitySignals are normalized reconciliation facts. The task/version
// absence flags alone are intentionally non-authoritative.
type AmbiguitySignals struct {
	MatchingOperationObserved bool
	ExpectedVersionObserved   bool
	ContradictoryEvidence     bool
	TaskAbsent                bool
	VersionAbsent             bool
	AbsenceProof              AbsenceProof
}

func ClassifyAmbiguity(signals AmbiguitySignals) AmbiguityOutcome {
	if signals.ContradictoryEvidence {
		return AmbiguityContradicted
	}
	if signals.ExpectedVersionObserved {
		return AmbiguityExpectedVersionObserved
	}
	if signals.MatchingOperationObserved {
		return AmbiguityMatchingOperationAdopted
	}
	if signals.AbsenceProof.IsSufficient() {
		return AmbiguityProvenAbsent
	}
	return AmbiguityInconclusive
}
