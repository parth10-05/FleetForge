package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
)

var ErrInvalidCampaignCreation = errors.New("invalid campaign creation command")

type CreateCampaignCommand struct {
	CampaignID, PolicyVersionID, ArtifactID, SelectionSeed string
	Selector                                               inventory.Selector
	Policy                                                 campaignplan.Policy
}

type CampaignCreationResult struct {
	CampaignID, CommandID string
	Revision              int64
	AcceptedAt            time.Time
}

type CampaignCreationStore interface {
	CreateCampaign(context.Context, access.Principal, CommandMeta, CreateCampaignCommand) (CampaignCreationResult, error)
}

type CampaignValidationStore interface {
	ValidateCampaign(context.Context, access.Principal, CommandMeta, string, int64) (CampaignCreationResult, error)
}

type CampaignService struct {
	store CampaignCreationStore
	now   func() time.Time
}

func NewCampaignService(store CampaignCreationStore, now func() time.Time) *CampaignService {
	if now == nil {
		now = time.Now
	}
	return &CampaignService{store: store, now: now}
}

func (service *CampaignService) CreateCampaign(ctx context.Context, principal access.Principal, meta CommandMeta, command CreateCampaignCommand) (CampaignCreationResult, error) {
	if service == nil || service.store == nil || meta.ClaimID == "" || meta.CommandID == "" || meta.CorrelationID == "" || meta.OwnerToken == "" || meta.OutboxEventID == "" || meta.DecisionInputID == "" || meta.Route != "/v1/campaigns" || meta.IdempotencyKey == "" || len(meta.CanonicalRequest) == 0 || command.CampaignID == "" || command.PolicyVersionID == "" || command.ArtifactID == "" || command.SelectionSeed == "" || len(command.Selector.Terms()) == 0 {
		return CampaignCreationResult{}, ErrInvalidCampaignCreation
	}
	if err := command.Policy.Validate(10_000); err != nil {
		return CampaignCreationResult{}, fmt.Errorf("%w: policy", ErrInvalidCampaignCreation)
	}
	if err := access.Authorize(principal, access.PermissionCreateCampaigns, service.now()); err != nil {
		return CampaignCreationResult{}, err
	}
	return service.store.CreateCampaign(ctx, principal, meta, command)
}

// ValidateCampaign begins asynchronous validation. It never selects targets
// in the request path or claims the campaign has reached READY.
func (service *CampaignService) ValidateCampaign(ctx context.Context, principal access.Principal, meta CommandMeta, campaignID string, expectedRevision int64) (CampaignCreationResult, error) {
	if service == nil || meta.ClaimID == "" || meta.CommandID == "" || meta.CorrelationID == "" || meta.OwnerToken == "" || meta.OutboxEventID == "" || meta.DecisionInputID == "" || meta.WorkID == "" || meta.Route != "/v1/campaigns/{campaign_id}:validate" || meta.IdempotencyKey == "" || len(meta.CanonicalRequest) == 0 || campaignID == "" || expectedRevision < 0 {
		return CampaignCreationResult{}, ErrInvalidCampaignCreation
	}
	store, ok := service.store.(CampaignValidationStore)
	if !ok {
		return CampaignCreationResult{}, fmt.Errorf("campaign validation persistence is unavailable")
	}
	if err := access.Authorize(principal, access.PermissionOperateCampaigns, service.now()); err != nil {
		return CampaignCreationResult{}, err
	}
	return store.ValidateCampaign(ctx, principal, meta, campaignID, expectedRevision)
}
