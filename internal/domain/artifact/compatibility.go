package artifact

import (
	"errors"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const CompatibilityPolicyV1 = "synthetic-family-exact-v1"

var ErrInvalidCompatibilityInput = errors.New("invalid compatibility input")

// CompatibilityDecision is a deterministic, persistable policy result. It
// intentionally does not inspect transport data or mutate artifact lifecycle.
type CompatibilityDecision struct {
	policyVersion string
	compatible    bool
	reason        string
}

func (decision CompatibilityDecision) PolicyVersion() string { return decision.policyVersion }
func (decision CompatibilityDecision) Compatible() bool      { return decision.compatible }
func (decision CompatibilityDecision) Reason() string        { return decision.reason }

// EvaluateCompatibility accepts only a verified artifact whose declared family
// exactly matches the normalized target family. V1 has no broad override.
func EvaluateCompatibility(policyVersion string, artifactState State, artifactFamily, targetFamily value.LabelValue) (CompatibilityDecision, error) {
	if policyVersion != CompatibilityPolicyV1 || artifactFamily.IsZero() || targetFamily.IsZero() {
		return CompatibilityDecision{}, ErrInvalidCompatibilityInput
	}
	if artifactState != StateVerified {
		return CompatibilityDecision{policyVersion: policyVersion, reason: "artifact_not_verified"}, nil
	}
	if artifactFamily.String() != targetFamily.String() {
		return CompatibilityDecision{policyVersion: policyVersion, reason: "target_family_mismatch"}, nil
	}
	return CompatibilityDecision{policyVersion: policyVersion, compatible: true, reason: "compatible"}, nil
}
