package artifact

import (
	"errors"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestCompatibilityRequiresVerifiedArtifactAndExactFamily(t *testing.T) {
	family, _ := value.ParseLabelValue("x11")
	other, _ := value.ParseLabelValue("x12")
	decision, err := EvaluateCompatibility(CompatibilityPolicyV1, StateVerified, family, family)
	if err != nil || !decision.Compatible() || decision.Reason() != "compatible" {
		t.Fatalf("verified exact family = %#v, %v", decision, err)
	}
	for _, state := range []State{StateInitiated, StateUploading, StateUploaded, StateVerifying, StateRejected, StateRetired, StateExpired} {
		decision, err := EvaluateCompatibility(CompatibilityPolicyV1, state, family, family)
		if err != nil || decision.Compatible() || decision.Reason() != "artifact_not_verified" {
			t.Fatalf("state %s = %#v, %v", state, decision, err)
		}
	}
	decision, err = EvaluateCompatibility(CompatibilityPolicyV1, StateVerified, family, other)
	if err != nil || decision.Compatible() || decision.Reason() != "target_family_mismatch" {
		t.Fatalf("mismatch = %#v, %v", decision, err)
	}
	if _, err := EvaluateCompatibility("other", StateVerified, family, family); !errors.Is(err, ErrInvalidCompatibilityInput) {
		t.Fatalf("invalid policy = %v", err)
	}
}
