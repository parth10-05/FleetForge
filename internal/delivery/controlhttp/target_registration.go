package controlhttp

import (
	"fmt"
	"maps"

	"github.com/parth10-05/fleetforge/internal/application/inventory"
	domaininventory "github.com/parth10-05/fleetforge/internal/domain/inventory"
)

func targetRegistrationRequest(targetID string, input TargetRegistration) (inventory.RegisterTargetRequest, error) {
	if targetID == "" || input.DisplayName == "" || input.EndpointReference == "" || input.TargetKey == "" || input.CredentialReference == "" || input.Labels == nil {
		return inventory.RegisterTargetRequest{}, fmt.Errorf("incomplete target registration")
	}
	request := inventory.RegisterTargetRequest{
		TargetID:            targetID,
		ExternalID:          input.DisplayName,
		Origin:              input.EndpointReference,
		TargetKey:           input.TargetKey,
		CredentialReference: input.CredentialReference,
		TransportPolicy:     domaininventory.SyntheticTransportPolicyV1,
		Labels:              maps.Clone(input.Labels),
	}
	if len(request.Labels) > 32 {
		return inventory.RegisterTargetRequest{}, fmt.Errorf("too many target labels")
	}
	return request, nil
}
