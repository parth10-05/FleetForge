package access

import (
	"testing"
	"time"
)

func TestMapGroupsToRolesAndAuthorization(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	roles, err := MapGroupsToRoles([]string{"fleetforge-operator"})
	if err != nil || len(roles) != 1 || roles[0] != RoleOperator {
		t.Fatalf("roles=%v err=%v", roles, err)
	}
	principal := Principal{Subject: "operator-1", Roles: roles, AuthenticatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute), IssuerClass: "local-synthetic-hs256"}
	if err := Authorize(principal, PermissionOperateCampaigns, now); err != nil {
		t.Fatalf("operator campaign authorization: %v", err)
	}
	if err := Authorize(principal, PermissionInitiateArtifacts, now); err == nil {
		t.Fatal("operator received admin artifact permission")
	}
}

func TestRecentAuthenticationAndGroupFailures(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	if _, err := MapGroupsToRoles([]string{"fleetforge-viewer", "fleetforge-viewer"}); err == nil {
		t.Fatal("duplicate group accepted")
	}
	if _, err := MapGroupsToRoles([]string{"unknown"}); err == nil {
		t.Fatal("unknown group accepted")
	}
	principal := Principal{Subject: "admin-1", Roles: []Role{RoleAdmin}, AuthenticatedAt: now.Add(-16 * time.Minute), ExpiresAt: now.Add(time.Minute), IssuerClass: "local-synthetic-hs256"}
	if err := RequireRecentAuthentication(principal, now, 15*time.Minute); err == nil {
		t.Fatal("stale authentication accepted")
	}
}
