package campaignplan

import (
	"bytes"
	"testing"
)

func FuzzBuildCanonicalPlanIsStable(f *testing.F) {
	f.Add("seed", uint8(2))
	f.Add("different-seed", uint8(5))
	f.Add("", uint8(1))
	f.Fuzz(func(t *testing.T, seed string, size uint8) {
		count := int(size%8) + 1
		candidates := make([]Candidate, 0, count)
		for index := 0; index < count; index++ {
			candidates = append(candidates, candidate(t, uuidFor(index+1), "site-a", "rack-"+string(rune('a'+index%3))))
		}
		canary := uint64(1)
		if count > 1 {
			canary = uint64(1 + int(size)%(count-1))
		}
		input := Input{Policy: validPolicy(Size{Unit: SizeUnitAbsolute, Value: canary}), Seed: seed, Algorithm: SelectionVersionV1, Candidates: candidates}
		first, firstErr := Build(input)
		second, secondErr := Build(input)
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("non-deterministic validity: %v / %v", firstErr, secondErr)
		}
		if firstErr == nil && (first.Hash != second.Hash || !bytes.Equal(first.PolicyJSON, second.PolicyJSON)) {
			t.Fatalf("non-deterministic plan: %#v / %#v", first, second)
		}
	})
}
