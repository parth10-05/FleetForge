package targetupdate

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestStateParserAcceptsEveryDeclaredState(t *testing.T) {
	for _, want := range AllStates() {
		got, err := ParseState(want.String())
		if err != nil || got != want {
			t.Fatalf("ParseState(%q) = %q, %v", want, got, err)
		}
		encoded, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("marshal %q: %v", want, err)
		}
		var raw string
		if err := json.Unmarshal(encoded, &raw); err != nil {
			t.Fatalf("unmarshal %q: %v", want, err)
		}
		if _, err := ParseState(raw); err != nil {
			t.Fatalf("round-trip parse %q: %v", raw, err)
		}
	}
}

func TestStateParserRejectsUnknownAndDoesNotExposeStateSlice(t *testing.T) {
	for _, raw := range []string{"", "pending", "WAITING_NETWORK"} {
		_, err := ParseState(raw)
		if !errors.Is(err, ErrUnknownState) {
			t.Fatalf("ParseState(%q) error = %v, want ErrUnknownState", raw, err)
		}
	}
	states := AllStates()
	states[0] = "MUTATED"
	if AllStates()[0] != StatePending {
		t.Fatal("AllStates exposed mutable registry storage")
	}
}
