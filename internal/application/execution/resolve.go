package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

// Resolution credentials are intentionally short lived: an ambiguity exit can
// release quarantine or authorize later work, so a long-lived token alone is
// insufficient administrative intent.
const maxResolutionAuthenticationAge = 15 * time.Minute

type AttentionResolver interface {
	ResolveAttention(context.Context, AdminResolutionCommand) (ResolutionResult, error)
}

// ResolveAttention verifies an administrative identity at command time. A
// bearer string is never persisted or treated as evidence; only the verified
// principal and the immutable resolution evidence reach the repository.
func ResolveAttention(ctx context.Context, verifier access.TokenVerifier, repository AttentionResolver, token string, command AdminResolutionCommand) (ResolutionResult, error) {
	if verifier == nil || repository == nil || token == "" || command.AttemptID == "" || command.Action == "" || command.Reason == "" || command.EvidenceFingerprint == "" || command.Now.IsZero() {
		return ResolutionResult{}, fmt.Errorf("invalid attention resolution request")
	}
	principal, err := verifier.VerifyPrincipal(ctx, token)
	if err != nil {
		return ResolutionResult{}, fmt.Errorf("verify resolution principal: %w", err)
	}
	if err := access.Authorize(principal, access.PermissionResolveAttention, command.Now); err != nil || access.RequireRecentAuthentication(principal, command.Now, maxResolutionAuthenticationAge) != nil {
		return ResolutionResult{}, fmt.Errorf("principal is not authorized to resolve firmware ambiguity")
	}
	command.AccessPrincipal = principal
	if command.Action == "RETRY_PROVEN_ABSENT" && (!command.JournalConfirmedAbsent || !command.LateCallerExcluded || command.LateCallerExclusionMethod == "" || command.ProofReference == "") {
		return ResolutionResult{}, fmt.Errorf("retry resolution lacks absence or late-caller exclusion proof")
	}
	if command.Action != "ADOPT" && command.Action != "RETRY_PROVEN_ABSENT" && command.Action != "TERMINAL" {
		return ResolutionResult{}, fmt.Errorf("unsupported attention resolution action")
	}
	if command.Action == "TERMINAL" && command.ReservationDisposition != "QUARANTINE" && command.ReservationDisposition != "RELEASE" {
		return ResolutionResult{}, fmt.Errorf("terminal attention resolution requires explicit reservation disposition")
	}
	if command.Action != "TERMINAL" && command.ReservationDisposition != "" {
		return ResolutionResult{}, fmt.Errorf("non-terminal attention resolution cannot change reservations")
	}
	if command.ReservationDisposition == "RELEASE" && command.ProofReference == "" {
		return ResolutionResult{}, fmt.Errorf("reservation release requires safe disposition proof")
	}
	return repository.ResolveAttention(ctx, command)
}
