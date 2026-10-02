package inventory

import (
	"context"
	"testing"

	domaininventory "github.com/parth10-05/fleetforge/internal/domain/inventory"
)

func TestRegisterTargetValidatesBeforeWriter(t *testing.T) {
	writer := &recordingRegistrationWriter{}
	service := New(writer)
	receipt, err := service.RegisterTarget(context.Background(), RegisterTargetRequest{TargetID: "00000000-0000-0000-3001-000000000001", ExternalID: "rack-a", Origin: "https://simulator.example", TargetKey: "rack-a", CredentialReference: "synthetic-test:rack-a", TransportPolicy: domaininventory.SyntheticTransportPolicyV1, Labels: map[string]string{"site": "west", "rack": "rack-a", "family": "synthetic"}})
	if err != nil || !receipt.Created || writer.registration.ExternalID() != "rack-a" {
		t.Fatalf("receipt=%+v writer=%+v err=%v", receipt, writer, err)
	}
	if _, err := service.RegisterTarget(context.Background(), RegisterTargetRequest{TargetID: "id", ExternalID: "rack-a", Origin: "https://user:password@simulator.example", TargetKey: "rack-a", CredentialReference: "synthetic-test:rack-a", TransportPolicy: domaininventory.SyntheticTransportPolicyV1, Labels: map[string]string{"site": "west", "rack": "rack-a"}}); err == nil || writer.calls != 1 {
		t.Fatalf("invalid registration err=%v calls=%d", err, writer.calls)
	}
}

type recordingRegistrationWriter struct {
	registration domaininventory.Registration
	calls        int
}

func (writer *recordingRegistrationWriter) Register(_ context.Context, id string, registration domaininventory.Registration) (RegistrationReceipt, error) {
	writer.registration, writer.calls = registration, writer.calls+1
	return RegistrationReceipt{TargetID: id, Created: true}, nil
}
