package inventory

import (
	"bytes"
	"errors"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestSelectorCanonicalizesAndMatchesExactLabels(t *testing.T) {
	selector, err := ParseSelector([]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"rack","value":"rack-a"},{"key":"site","value":"west"},{"key":"family","value":"x1"}]}`))
	if err != nil {
		t.Fatalf("ParseSelector() error = %v", err)
	}
	if got, want := string(selector.CanonicalJSON()), `{"version":"fleetforge.selector/v1","all":[{"key":"family","value":"x1"},{"key":"rack","value":"rack-a"},{"key":"site","value":"west"}]}`; got != want {
		t.Fatalf("CanonicalJSON() = %s, want %s", got, want)
	}
	labels, err := value.NewFailureDomainLabels(map[string]string{"site": "west", "rack": "rack-a", "family": "x1"})
	if err != nil || !selector.Matches(labels) {
		t.Fatalf("selector did not match: %v", err)
	}
	if !bytes.Equal(selector.CanonicalJSON(), selector.CanonicalJSON()) {
		t.Fatal("canonical selector must be stable")
	}
}

func TestSelectorRejectsOperatorsUnknownAndDuplicateKeys(t *testing.T) {
	for _, raw := range []string{`[]`, `{"version":"fleetforge.selector/v1","all":[]}`, `{"version":"fleetforge.selector/v1","all":[{"key":"site","value":{"eq":"west"}}]}`, `{"version":"fleetforge.selector/v1","all":[{"key":"zone","value":"west"}]}`, `{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"},{"key":"site","value":"east"}]}`} {
		_, err := ParseSelector([]byte(raw))
		if !errors.Is(err, ErrInvalidSelector) {
			t.Fatalf("ParseSelector(%s) = %v", raw, err)
		}
	}
}
