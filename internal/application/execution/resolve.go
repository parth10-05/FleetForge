package execution

import (
	"context"
	"fmt"
	"slices"
)

const resolutionRole = "firmware:resolve"

type AttentionResolver interface {
	ResolveAttention(context.Context, AdminResolutionCommand) (ResolutionResult, error)
}

// ResolveAttention verifies an administrative identity at command time. A
// bearer string is never persisted or treated as evidence; only the verified
// principal and the immutable resolution evidence reach the repository.
func ResolveAttention(ctx context.Context, verifier TokenVerifier, repository AttentionResolver, token string, command AdminResolutionCommand) (ResolutionResult, error) {
	if verifier == nil || repository == nil || token == "" || command.AttemptID == "" || command.Action == "" || command.Reason == "" || command.EvidenceFingerprint == "" || command.Now.IsZero() {
		return ResolutionResult{}, fmt.Errorf("invalid attention resolution request")
	}
	principal, err := verifier.VerifyAccessToken(ctx, token)
	if err != nil {
		return ResolutionResult{}, fmt.Errorf("verify resolution principal: %w", err)
	}
	if !principal.Verified || principal.Subject == "" || principal.ExpiresAt.IsZero() || !principal.ExpiresAt.After(command.Now) || !slices.Contains(principal.Roles, resolutionRole) {
		return ResolutionResult{}, fmt.Errorf("principal is not authorized to resolve firmware ambiguity")
	}
	command.Principal = principal
	if command.Action == "RETRY_PROVEN_ABSENT" && (!command.JournalConfirmedAbsent || !command.LateCallerExcluded || command.LateCallerExclusionMethod == "" || command.ProofReference == "") {
		return ResolutionResult{}, fmt.Errorf("retry resolution lacks absence or late-caller exclusion proof")
	}
	if command.Action != "ADOPT" && command.Action != "RETRY_PROVEN_ABSENT" && command.Action != "TERMINAL" {
		return ResolutionResult{}, fmt.Errorf("unsupported attention resolution action")
	}
	return repository.ResolveAttention(ctx, command)
}
