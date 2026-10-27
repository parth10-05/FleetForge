// Package control coordinates authenticated API commands through provider-neutral ports.
package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
)

var (
	ErrInvalidTargetRegistration = errors.New("invalid target registration command")
	ErrTargetNotFound            = errors.New("target projection not found")
	ErrTargetRevisionConflict    = errors.New("target revision conflict")
)

type CommandMeta struct {
	ClaimID, CommandID, CorrelationID, OwnerToken string
	OutboxEventID, DecisionInputID, WorkID        string
	Route, IdempotencyKey                         string
	CanonicalRequest                              []byte
}

type RegisterTargetCommand struct {
	TargetID, ExternalID, Origin, TargetKey string
	CredentialReference                     string
	Labels                                  map[string]string
}

type TargetRegistrationResult struct {
	TargetID, CommandID string
	Revision            int64
	AcceptedAt          time.Time
}

type TargetRegistrationStore interface {
	RegisterTarget(context.Context, access.Principal, CommandMeta, string, inventory.Registration) (TargetRegistrationResult, error)
}

type TargetDiscoveryStore interface {
	DiscoverTarget(context.Context, access.Principal, CommandMeta, string, int64) (TargetRegistrationResult, error)
}

type TargetProjectionReader interface {
	GetTarget(context.Context, string) (TargetRegistrationResult, string, error)
}

type TargetService struct {
	store TargetRegistrationStore
	now   func() time.Time
}

func NewTargetService(store TargetRegistrationStore, now func() time.Time) *TargetService {
	if now == nil {
		now = time.Now
	}
	return &TargetService{store: store, now: now}
}

func (service *TargetService) RegisterTarget(ctx context.Context, principal access.Principal, meta CommandMeta, command RegisterTargetCommand) (TargetRegistrationResult, error) {
	if service == nil || service.store == nil || meta.ClaimID == "" || meta.CommandID == "" || meta.CorrelationID == "" || meta.OwnerToken == "" || meta.OutboxEventID == "" || meta.DecisionInputID == "" || meta.Route != "/v1/targets" || meta.IdempotencyKey == "" || len(meta.CanonicalRequest) == 0 || command.TargetID == "" {
		return TargetRegistrationResult{}, ErrInvalidTargetRegistration
	}
	if err := access.Authorize(principal, access.PermissionRegisterTargets, service.now()); err != nil {
		return TargetRegistrationResult{}, err
	}
	registration, err := inventory.NewRegistration(command.ExternalID, command.Origin, command.TargetKey, inventory.SyntheticContractV1, command.CredentialReference, inventory.SyntheticTransportPolicyV1, command.Labels)
	if err != nil {
		return TargetRegistrationResult{}, fmt.Errorf("%w: %v", ErrInvalidTargetRegistration, err)
	}
	return service.store.RegisterTarget(ctx, principal, meta, command.TargetID, registration)
}

func (service *TargetService) GetTarget(ctx context.Context, principal access.Principal, targetID string) (TargetRegistrationResult, string, error) {
	if service == nil || service.store == nil || targetID == "" {
		return TargetRegistrationResult{}, "", ErrInvalidTargetRegistration
	}
	if err := access.Authorize(principal, access.PermissionReadTargets, service.now()); err != nil {
		return TargetRegistrationResult{}, "", err
	}
	reader, ok := service.store.(TargetProjectionReader)
	if !ok {
		return TargetRegistrationResult{}, "", fmt.Errorf("target projection persistence is unavailable")
	}
	return reader.GetTarget(ctx, targetID)
}

func (service *TargetService) DiscoverTarget(ctx context.Context, principal access.Principal, meta CommandMeta, targetID string, expectedRevision int64) (TargetRegistrationResult, error) {
	if service == nil || service.store == nil || meta.ClaimID == "" || meta.CommandID == "" || meta.CorrelationID == "" || meta.OwnerToken == "" || meta.OutboxEventID == "" || meta.DecisionInputID == "" || meta.WorkID == "" || meta.Route != "/v1/targets/{target_id}:discover" || meta.IdempotencyKey == "" || len(meta.CanonicalRequest) == 0 || targetID == "" || expectedRevision < 0 {
		return TargetRegistrationResult{}, ErrInvalidTargetRegistration
	}
	if err := access.Authorize(principal, access.PermissionDiscoverTargets, service.now()); err != nil {
		return TargetRegistrationResult{}, err
	}
	store, ok := service.store.(TargetDiscoveryStore)
	if !ok {
		return TargetRegistrationResult{}, fmt.Errorf("target discovery persistence is unavailable")
	}
	return store.DiscoverTarget(ctx, principal, meta, targetID, expectedRevision)
}
