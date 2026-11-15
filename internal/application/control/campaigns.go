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

var (
	ErrInvalidCampaignCreation  = errors.New("invalid campaign creation command")
	ErrCampaignRevisionConflict = errors.New("campaign revision conflict")
	ErrCampaignCommandConflict  = errors.New("campaign command conflict")
	ErrCampaignNotFound         = errors.New("campaign projection not found")
)

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

type CampaignStartStore interface {
	StartCampaign(context.Context, access.Principal, CommandMeta, string, int64) (CampaignCreationResult, error)
}

type CampaignPauseStore interface {
	PauseCampaign(context.Context, access.Principal, CommandMeta, string, int64) (CampaignCreationResult, error)
}

type CampaignProjection struct {
	CampaignID string
	State      string
	Revision   int64
	AsOf       time.Time
}

type CampaignProjectionReader interface {
	GetCampaign(context.Context, string) (CampaignProjection, error)
}

type CampaignTargetPage struct {
	Items        []TargetProjection
	NextTargetID string
}

type CampaignTargetProjectionLister interface {
	ListCampaignTargets(context.Context, string, string, int) (CampaignTargetPage, error)
}

type CampaignAuditRecord struct {
	Action, Outcome string
	OccurredAt      time.Time
}

type CampaignAuditPage struct {
	Items                 []CampaignAuditRecord
	NextAggregateRevision int64
	NextEventSequence     int64
	HasNext               bool
}

type CampaignAuditLister interface {
	ListCampaignAudit(context.Context, string, int64, int64, int) (CampaignAuditPage, error)
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

func (service *CampaignService) GetCampaign(ctx context.Context, principal access.Principal, campaignID string) (CampaignProjection, error) {
	if service == nil || service.store == nil || campaignID == "" {
		return CampaignProjection{}, ErrInvalidCampaignCreation
	}
	if err := access.Authorize(principal, access.PermissionReadTargets, service.now()); err != nil {
		return CampaignProjection{}, err
	}
	reader, ok := service.store.(CampaignProjectionReader)
	if !ok {
		return CampaignProjection{}, fmt.Errorf("campaign projection persistence is unavailable")
	}
	return reader.GetCampaign(ctx, campaignID)
}

func (service *CampaignService) ListCampaignTargets(ctx context.Context, principal access.Principal, campaignID, afterID string, pageSize int) (CampaignTargetPage, error) {
	if service == nil || service.store == nil || campaignID == "" || pageSize < 1 || pageSize > 100 {
		return CampaignTargetPage{}, ErrInvalidCampaignCreation
	}
	if err := access.Authorize(principal, access.PermissionReadTargets, service.now()); err != nil {
		return CampaignTargetPage{}, err
	}
	lister, ok := service.store.(CampaignTargetProjectionLister)
	if !ok {
		return CampaignTargetPage{}, fmt.Errorf("campaign target projection persistence is unavailable")
	}
	return lister.ListCampaignTargets(ctx, campaignID, afterID, pageSize)
}

func (service *CampaignService) ListCampaignAudit(ctx context.Context, principal access.Principal, campaignID string, afterRevision, afterSequence int64, pageSize int) (CampaignAuditPage, error) {
	if service == nil || service.store == nil || campaignID == "" || afterRevision < 0 || afterSequence < -1 || pageSize < 1 || pageSize > 100 {
		return CampaignAuditPage{}, ErrInvalidCampaignCreation
	}
	if err := access.Authorize(principal, access.PermissionReadTargets, service.now()); err != nil {
		return CampaignAuditPage{}, err
	}
	lister, ok := service.store.(CampaignAuditLister)
	if !ok {
		return CampaignAuditPage{}, fmt.Errorf("campaign audit persistence is unavailable")
	}
	return lister.ListCampaignAudit(ctx, campaignID, afterRevision, afterSequence, pageSize)
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

func (service *CampaignService) StartCampaign(ctx context.Context, principal access.Principal, meta CommandMeta, campaignID string, expectedRevision int64) (CampaignCreationResult, error) {
	if service == nil || meta.ClaimID == "" || meta.CommandID == "" || meta.CorrelationID == "" || meta.OwnerToken == "" || meta.OutboxEventID == "" || meta.DecisionInputID == "" || meta.Route != "/v1/campaigns/{campaign_id}:start" || meta.IdempotencyKey == "" || len(meta.CanonicalRequest) == 0 || campaignID == "" || expectedRevision < 0 {
		return CampaignCreationResult{}, ErrInvalidCampaignCreation
	}
	store, ok := service.store.(CampaignStartStore)
	if !ok {
		return CampaignCreationResult{}, fmt.Errorf("campaign start persistence is unavailable")
	}
	if err := access.Authorize(principal, access.PermissionOperateCampaigns, service.now()); err != nil {
		return CampaignCreationResult{}, err
	}
	return store.StartCampaign(ctx, principal, meta, campaignID, expectedRevision)
}

func (service *CampaignService) PauseCampaign(ctx context.Context, principal access.Principal, meta CommandMeta, campaignID string, expectedRevision int64) (CampaignCreationResult, error) {
	if service == nil || meta.ClaimID == "" || meta.CommandID == "" || meta.CorrelationID == "" || meta.OwnerToken == "" || meta.OutboxEventID == "" || meta.DecisionInputID == "" || meta.Route != "/v1/campaigns/{campaign_id}:pause" || meta.IdempotencyKey == "" || len(meta.CanonicalRequest) == 0 || campaignID == "" || expectedRevision < 0 {
		return CampaignCreationResult{}, ErrInvalidCampaignCreation
	}
	store, ok := service.store.(CampaignPauseStore)
	if !ok {
		return CampaignCreationResult{}, fmt.Errorf("campaign pause persistence is unavailable")
	}
	if err := access.Authorize(principal, access.PermissionOperateCampaigns, service.now()); err != nil {
		return CampaignCreationResult{}, err
	}
	return store.PauseCampaign(ctx, principal, meta, campaignID, expectedRevision)
}
