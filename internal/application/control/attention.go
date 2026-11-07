package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

var (
	ErrInvalidAttentionResolution = errors.New("invalid attention resolution command")
	ErrAttentionRevisionConflict  = errors.New("attention resolution revision conflict")
	ErrAttentionCommandConflict   = errors.New("attention resolution command conflict")
)

type AttentionResolutionCommand struct {
	Action, ReasonCode, Reason, Reference, ReservationDisposition, LateCallerExclusionMethod string
	JournalConfirmedAbsent, LateCallerExcluded                                               bool
}

type AttentionResolutionResult struct {
	TargetUpdateID, CommandID string
	Revision                  int64
	AcceptedAt                time.Time
}

type AttentionResolutionStore interface {
	ResolveAttention(context.Context, access.Principal, CommandMeta, string, int64, AttentionResolutionCommand) (AttentionResolutionResult, error)
}

type AttentionService struct {
	store AttentionResolutionStore
	now   func() time.Time
}

func NewAttentionService(store AttentionResolutionStore, now func() time.Time) *AttentionService {
	if now == nil {
		now = time.Now
	}
	return &AttentionService{store: store, now: now}
}

func (service *AttentionService) ResolveAttention(ctx context.Context, principal access.Principal, meta CommandMeta, targetUpdateID string, expectedRevision int64, command AttentionResolutionCommand) (AttentionResolutionResult, error) {
	if service == nil || service.store == nil || meta.ClaimID == "" || meta.CommandID == "" || meta.CorrelationID == "" || meta.OwnerToken == "" || meta.Route != "/v1/target-updates/{target_update_id}:resolve-attention" || meta.IdempotencyKey == "" || len(meta.CanonicalRequest) == 0 || targetUpdateID == "" || expectedRevision < 0 || command.Action == "" || command.ReasonCode == "" || command.Reason == "" {
		return AttentionResolutionResult{}, ErrInvalidAttentionResolution
	}
	now := service.now()
	if err := access.Authorize(principal, access.PermissionResolveAttention, now); err != nil {
		return AttentionResolutionResult{}, err
	}
	if err := access.RequireRecentAuthentication(principal, now, 15*time.Minute); err != nil {
		return AttentionResolutionResult{}, fmt.Errorf("%w: recent authentication", ErrInvalidAttentionResolution)
	}
	return service.store.ResolveAttention(ctx, principal, meta, targetUpdateID, expectedRevision, command)
}
