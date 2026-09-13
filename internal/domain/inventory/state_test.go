package inventory

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAdministrativeStateParserAcceptsEveryDeclaredState(t *testing.T) {
	for _, want := range AllAdministrativeStates() {
		got, err := ParseAdministrativeState(want.String())
		if err != nil || got != want {
			t.Fatalf("ParseAdministrativeState(%q) = %q, %v", want, got, err)
		}
		encoded, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("marshal %q: %v", want, err)
		}
		var raw string
		if err := json.Unmarshal(encoded, &raw); err != nil {
			t.Fatalf("unmarshal %q: %v", want, err)
		}
		if _, err := ParseAdministrativeState(raw); err != nil {
			t.Fatalf("round-trip parse %q: %v", raw, err)
		}
	}
}

func TestAdministrativeStateParserRejectsUnknownAndDoesNotExposeStateSlice(t *testing.T) {
	for _, raw := range []string{"", "active", "RETIRED"} {
		_, err := ParseAdministrativeState(raw)
		if !errors.Is(err, ErrUnknownAdministrativeState) {
			t.Fatalf("ParseAdministrativeState(%q) error = %v, want ErrUnknownAdministrativeState", raw, err)
		}
	}
	states := AllAdministrativeStates()
	states[0] = "MUTATED"
	if AllAdministrativeStates()[0] != AdministrativeStateActive {
		t.Fatal("AllAdministrativeStates exposed mutable registry storage")
	}
}
