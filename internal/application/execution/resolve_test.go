package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

type verifierFunc func(context.Context, string) (VerifiedPrincipal, error)

func (f verifierFunc) VerifyAccessToken(ctx context.Context, token string) (VerifiedPrincipal, error) {
	return f(ctx, token)
}

type resolutionRepo struct{ calls int }

func (r *resolutionRepo) ResolveAttention(context.Context, AdminResolutionCommand) (ResolutionResult, error) {
	r.calls++
	return ResolutionResult{TargetUpdateID: "update"}, nil
}

func TestResolveAttentionRequiresVerifiedRoleAndAbsenceProof(t *testing.T) {
	now := time.Now().UTC()
	verifier := verifierFunc(func(context.Context, string) (VerifiedPrincipal, error) {
		return VerifiedPrincipal{Subject: "admin", Roles: []string{resolutionRole}, Verified: true, AuthenticatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}, nil
	})
	repository := &resolutionRepo{}
	command := AdminResolutionCommand{AttemptID: "attempt", Action: "RETRY_PROVEN_ABSENT", Reason: "verified", EvidenceFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Now: now}
	if _, err := ResolveAttention(context.Background(), verifier, repository, "token", command); err == nil || repository.calls != 0 {
		t.Fatalf("unproven retry err=%v calls=%d", err, repository.calls)
	}
	command.JournalConfirmedAbsent, command.LateCallerExcluded, command.LateCallerExclusionMethod, command.ProofReference = true, true, "worker-lease-revoked", "proof-1"
	if _, err := ResolveAttention(context.Background(), verifier, repository, "token", command); err != nil || repository.calls != 1 {
		t.Fatalf("proven retry err=%v calls=%d", err, repository.calls)
	}
}

func TestResolveAttentionRequiresRecentAuthentication(t *testing.T) {
	now := time.Now().UTC()
	verifier := verifierFunc(func(context.Context, string) (VerifiedPrincipal, error) {
		return VerifiedPrincipal{Subject: "admin", Roles: []string{resolutionRole}, Verified: true, AuthenticatedAt: now.Add(-maxResolutionAuthenticationAge - time.Second), ExpiresAt: now.Add(time.Minute)}, nil
	})
	_, err := ResolveAttention(context.Background(), verifier, &resolutionRepo{}, "token", AdminResolutionCommand{AttemptID: "attempt", Action: "ADOPT", Reason: "verified", EvidenceFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Now: now})
	if err == nil {
		t.Fatal("stale authentication was accepted for ambiguity resolution")
	}
}

func TestResolveAttentionRejectsUnverifiedPrincipal(t *testing.T) {
	verifier := verifierFunc(func(context.Context, string) (VerifiedPrincipal, error) {
		return VerifiedPrincipal{}, errors.New("invalid token")
	})
	_, err := ResolveAttention(context.Background(), verifier, &resolutionRepo{}, "token", AdminResolutionCommand{AttemptID: "attempt", Action: "ADOPT", Reason: "verified", EvidenceFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Now: time.Now()})
	if err == nil {
		t.Fatal("unverified principal was accepted")
	}
}
