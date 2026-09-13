package wave

import (
	"github.com/parth10-05/fleetforge/internal/domain/value"
	"testing"
)

func ordinal(t *testing.T, n int64) value.WaveOrdinal {
	t.Helper()
	v, e := value.NewWaveOrdinal(n)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestCanReleaseRequiresImmediatePassedPredecessor(t *testing.T) {
	first := Wave{Ordinal: ordinal(t, 1)}
	if !CanRelease(first, nil).Allowed {
		t.Fatal("first wave")
	}
	second := Wave{Ordinal: ordinal(t, 2)}
	if CanRelease(second, nil).Allowed {
		t.Fatal("missing predecessor")
	}
	previous := Wave{Ordinal: ordinal(t, 1), Terminal: true, Evaluation: EvaluationPassed}
	if !CanRelease(second, &previous).Allowed {
		t.Fatal("valid predecessor")
	}
	previous.Evaluation = EvaluationFailed
	if CanRelease(second, &previous).Allowed {
		t.Fatal("failed predecessor")
	}
}
