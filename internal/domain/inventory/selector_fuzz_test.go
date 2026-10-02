package inventory

import (
	"bytes"
	"testing"
)

func FuzzSelectorCanonicalRoundTrip(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]}`),
		[]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"rack","value":"a"},{"key":"site","value":"west"}]}`),
		[]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":{"eq":"west"}}]}`),
		[]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"},{"key":"site","value":"east"}]}`),
		[]byte("\x00{}"),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		selector, err := ParseSelector(raw)
		if err != nil {
			return
		}
		if terms := selector.Terms(); len(terms) == 0 || len(terms) > maxSelectorTerms {
			t.Fatalf("accepted selector has %d terms", len(terms))
		}
		canonical := selector.CanonicalJSON()
		roundTrip, err := ParseSelector(canonical)
		if err != nil || !bytes.Equal(canonical, roundTrip.CanonicalJSON()) {
			t.Fatalf("canonical selector does not round trip: input=%q canonical=%q err=%v", raw, canonical, err)
		}
	})
}
