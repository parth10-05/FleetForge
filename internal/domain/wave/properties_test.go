package wave

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestWaveReleaseRequiresExactSuccessfulPredecessor(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ordinal := rapid.Int64Range(1, 100).Draw(t, "ordinal")
		next := Wave{Ordinal: mustOrdinal(t, ordinal), Released: rapid.Bool().Draw(t, "released")}
		var previous *Wave
		if ordinal > 1 && rapid.Bool().Draw(t, "has_previous") {
			previousOrdinal := rapid.Int64Range(1, 100).Draw(t, "previous_ordinal")
			previous = &Wave{Ordinal: mustOrdinal(t, previousOrdinal), Terminal: rapid.Bool().Draw(t, "terminal"), Evaluation: rapid.SampledFrom([]Evaluation{EvaluationPending, EvaluationPassed, EvaluationFailed}).Draw(t, "evaluation")}
		}
		got := CanRelease(next, previous)
		want := !next.Released && (ordinal == 1 || previous != nil && previous.Ordinal.Uint64()+1 == uint64(ordinal) && previous.Terminal && previous.Evaluation == EvaluationPassed)
		if got.Allowed != want {
			t.Fatalf("CanRelease(%#v, %#v) = %#v, want %t", next, previous, got, want)
		}
	})
}

type testReporter interface {
	Helper()
	Fatal(args ...any)
}

func mustOrdinal(t testReporter, raw int64) value.WaveOrdinal {
	t.Helper()
	ordinal, err := value.NewWaveOrdinal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ordinal
}
