// Package inventory coordinates transport-neutral target registration.
package inventory

import (
	"context"
	"fmt"

	domaininventory "github.com/parth10-05/fleetforge/internal/domain/inventory"
)

type RegistrationReceipt struct {
	TargetID string
	Created  bool
}

type RegistrationWriter interface {
	Register(context.Context, string, domaininventory.Registration) (RegistrationReceipt, error)
}

type RegisterTargetRequest struct {
	TargetID, ExternalID, Origin, TargetKey string
	CredentialReference                     string
	TransportPolicy                         string
	Labels                                  map[string]string
}

type Service struct{ registrations RegistrationWriter }

func New(registrations RegistrationWriter) *Service { return &Service{registrations: registrations} }

// RegisterTarget validates the complete target identity before handing it to
// the durable writer. It accepts a credential reference only; no credential
// value can enter the application command.
func (service *Service) RegisterTarget(ctx context.Context, request RegisterTargetRequest) (RegistrationReceipt, error) {
	if service == nil || service.registrations == nil || request.TargetID == "" {
		return RegistrationReceipt{}, fmt.Errorf("invalid target registration request")
	}
	registration, err := domaininventory.NewRegistration(request.ExternalID, request.Origin, request.TargetKey, domaininventory.SyntheticContractV1, request.CredentialReference, request.TransportPolicy, request.Labels)
	if err != nil {
		return RegistrationReceipt{}, fmt.Errorf("validate target registration: %w", err)
	}
	receipt, err := service.registrations.Register(ctx, request.TargetID, registration)
	if err != nil {
		return RegistrationReceipt{}, fmt.Errorf("persist target registration: %w", err)
	}
	return receipt, nil
}
