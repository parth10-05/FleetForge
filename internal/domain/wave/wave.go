package wave

import "github.com/parth10-05/fleetforge/internal/domain/value"

type Evaluation string

const (
	EvaluationPending Evaluation = "PENDING"
	EvaluationPassed  Evaluation = "PASSED"
	EvaluationFailed  Evaluation = "FAILED"
)

type Wave struct {
	ID         value.WaveID
	Ordinal    value.WaveOrdinal
	Terminal   bool
	Evaluation Evaluation
	Released   bool
}
type ReleaseDecision struct {
	Allowed bool
	Reason  string
}

func CanRelease(next Wave, previous *Wave) ReleaseDecision {
	if next.Released {
		return ReleaseDecision{Reason: "wave already released"}
	}
	if next.Ordinal.Uint64() == 1 {
		return ReleaseDecision{Allowed: true}
	}
	if previous == nil || previous.Ordinal.Uint64()+1 != next.Ordinal.Uint64() {
		return ReleaseDecision{Reason: "immediate predecessor missing"}
	}
	if !previous.Terminal || previous.Evaluation != EvaluationPassed {
		return ReleaseDecision{Reason: "predecessor not terminal and passed"}
	}
	return ReleaseDecision{Allowed: true}
}
