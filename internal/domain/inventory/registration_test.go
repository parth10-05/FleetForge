package inventory

import (
	"errors"
	"testing"
)

func TestRegistrationCanonicalizesEquivalentLabelsIntoOneSemanticHash(t *testing.T) {
	first, err := NewRegistration("rack-a-bmc-01", "https://simulator.example", "rack-a-bmc-01", SyntheticContractV1, "synthetic-test:rack-a", SyntheticTransportPolicyV1, map[string]string{"site": "iad", "rack": "a-01", "group": "canary"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRegistration("rack-a-bmc-01", "HTTPS://SIMULATOR.EXAMPLE/", "rack-a-bmc-01", SyntheticContractV1, "synthetic-test:rack-a", SyntheticTransportPolicyV1, map[string]string{"GROUP": "canary", "rack": "a-01", "site": "iad"})
	if err != nil {
		t.Fatal(err)
	}
	if first.SemanticHash() != second.SemanticHash() {
		t.Fatalf("equivalent registration hashes differ: %s and %s", first.SemanticHash().String(), second.SemanticHash().String())
	}
	changed, err := NewRegistration("rack-a-bmc-01", "https://other.example", "rack-a-bmc-01", SyntheticContractV1, "synthetic-test:rack-a", SyntheticTransportPolicyV1, map[string]string{"site": "iad", "rack": "a-01", "group": "canary"})
	if err != nil {
		t.Fatal(err)
	}
	if first.SemanticHash() == changed.SemanticHash() {
		t.Fatal("changed endpoint did not change the registration hash")
	}
}

func TestRegistrationRejectsSecretShapedOrIncompleteInput(t *testing.T) {
	valid := map[string]string{"site": "iad", "rack": "a-01"}
	for _, input := range []struct {
		externalID, reference, policy string
		labels                        map[string]string
	}{
		{externalID: "rack-a", reference: "aws-sm:secret/target", policy: SyntheticTransportPolicyV1, labels: valid},
		{externalID: "rack-a", reference: "synthetic-test:bad/name", policy: SyntheticTransportPolicyV1, labels: valid},
		{externalID: "rack-a\nsecret", reference: "synthetic-test:rack-a", policy: SyntheticTransportPolicyV1, labels: valid},
		{externalID: "rack-a", reference: "synthetic-test:rack-a", policy: "any", labels: valid},
		{externalID: "rack-a", reference: "synthetic-test:rack-a", policy: SyntheticTransportPolicyV1, labels: map[string]string{"site": "iad"}},
	} {
		_, err := NewRegistration(input.externalID, "https://simulator.example", "rack-a", SyntheticContractV1, input.reference, input.policy, input.labels)
		if !errors.Is(err, ErrInvalidRegistration) {
			t.Fatalf("NewRegistration(%+v) error = %v", input, err)
		}
	}
}
