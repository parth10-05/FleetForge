package localoidc

import (
	"context"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

func TestIssuerMintsExactRoleTokenAcceptedBySharedVerifier(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	issuer := Issuer{Issuer: "fleetforge-demo", Audience: "fleetforge-control", Key: key, Now: func() time.Time { return now }}
	token, err := issuer.Mint(MintRequest{Subject: "demo-admin", Roles: []access.Role{access.RoleAdmin}, TTL: 5 * time.Minute})
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	verifier := Verifier{Issuer: "fleetforge-demo", Audience: "fleetforge-control", Key: key, RecentAuth: 15 * time.Minute, Now: func() time.Time { return now }}
	principal, err := verifier.VerifyPrincipal(context.Background(), token)
	if err != nil || !principal.HasRole(access.RoleAdmin) {
		t.Fatalf("principal=%+v err=%v", principal, err)
	}
}

func TestIssuerRejectsUnsafeMintRequests(t *testing.T) {
	issuer := Issuer{Issuer: "issuer", Audience: "aud", Key: []byte("01234567890123456789012345678901")}
	if _, err := issuer.Mint(MintRequest{Subject: "x", Roles: []access.Role{access.RoleAdmin}, TTL: 2 * time.Hour}); err == nil {
		t.Fatal("oversized token lifetime accepted")
	}
	if _, err := issuer.Mint(MintRequest{Subject: "x", Roles: []access.Role{access.Role("unknown")}, TTL: time.Minute}); err == nil {
		t.Fatal("unknown role accepted")
	}
}
