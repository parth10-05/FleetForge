package preflight

import (
	"strings"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestSnapshotHashIsStableAcrossInputOrdering(t *testing.T) {
	selector, _ := inventory.ParseSelector([]byte(`{"site":"west"}`))
	digest, _ := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	object, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "firmware", "objects/a", "v1", 42, digest)
	artifactID, _ := value.ParseArtifactID("11111111-1111-4111-8111-111111111111")
	left := testTarget(t, "22222222-2222-4222-8222-222222222222", "rack-b")
	right := testTarget(t, "33333333-3333-4333-8333-333333333333", "rack-a")
	first, err := NewSnapshot(selector, artifactID, object, []Target{left, right})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSnapshot(selector, artifactID, object, []Target{right, left})
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash() != second.Hash() {
		t.Fatalf("hashes differ: %s %s", first.Hash().String(), second.Hash().String())
	}
	if first.Targets()[0].ID.String() != left.ID.String() {
		t.Fatal("targets were not canonically ordered")
	}
}

func testTarget(t *testing.T, id, rack string) Target {
	t.Helper()
	parsedID, _ := value.ParseTargetID(id)
	labels, err := value.NewFailureDomainLabels(map[string]string{"site": "west", "rack": rack, "family": "x11"})
	if err != nil {
		t.Fatal(err)
	}
	family, _ := value.ParseLabelValue("x11")
	decision, err := artifact.EvaluateCompatibility(artifact.CompatibilityPolicyV1, artifact.StateVerified, family, family)
	if err != nil {
		t.Fatal(err)
	}
	return Target{ID: parsedID, Revision: 1, Labels: labels, Family: family, Compatibility: decision}
}
