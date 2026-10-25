package controlhttp

import "testing"

func TestTargetRegistrationRequestMapsOnlyExplicitTransportFields(t *testing.T) {
	input := TargetRegistration{DisplayName: "rack-a-node-1", EndpointReference: "https://127.0.0.1:9443", TargetKey: "node-1", CredentialReference: "synthetic-test:node-1", Labels: map[string]string{"site": "iad", "rack": "r-01", "family": "x1"}}
	request, err := targetRegistrationRequest("00000000-0000-0000-0000-000000000001", input)
	if err != nil || request.ExternalID != input.DisplayName || request.TargetKey != input.TargetKey || request.TransportPolicy != "synthetic-allowlisted-v1" {
		t.Fatalf("request=%+v err=%v", request, err)
	}
	input.Labels["site"] = "changed"
	if request.Labels["site"] != "iad" {
		t.Fatal("labels were not defensively copied")
	}
}

func TestTargetRegistrationRequestRejectsMissingIdentity(t *testing.T) {
	if _, err := targetRegistrationRequest("id", TargetRegistration{DisplayName: "target", EndpointReference: "https://127.0.0.1", TargetKey: "target", CredentialReference: "synthetic-test:target"}); err == nil {
		t.Fatal("missing labels accepted")
	}
}
