// Package access defines provider-neutral authenticated identity data for
// FleetForge application operations. Provider tokens and claim names must not
// cross this boundary.
package access

import (
	"fmt"
	"slices"
	"time"
)

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

// Principal contains only the identity facts application code may trust after
// a platform verifier has completed token validation.
type Principal struct {
	Subject                    string
	Roles                      []Role
	AuthenticatedAt, ExpiresAt time.Time
	IssuerClass                string
}

func (principal Principal) Valid(now time.Time) bool {
	return principal.Subject != "" && principal.IssuerClass != "" &&
		!principal.AuthenticatedAt.IsZero() && !principal.AuthenticatedAt.After(now) &&
		!principal.ExpiresAt.IsZero() && principal.ExpiresAt.After(now)
}

func (principal Principal) HasRole(want Role) bool {
	return slices.Contains(principal.Roles, want)
}

// MapGroupsToRoles accepts only the exact local synthetic V1 group names.
// Unknown groups never grant permission; malformed or duplicated values are
// rejected so the verifier cannot silently normalize ambiguous identity data.
func MapGroupsToRoles(groups []string) ([]Role, error) {
	if len(groups) > 3 {
		return nil, fmt.Errorf("too many identity groups")
	}
	roles := make([]Role, 0, len(groups))
	seen := make(map[Role]struct{}, len(groups))
	for _, group := range groups {
		var role Role
		switch group {
		case "fleetforge-viewer":
			role = RoleViewer
		case "fleetforge-operator":
			role = RoleOperator
		case "fleetforge-admin":
			role = RoleAdmin
		case "":
			return nil, fmt.Errorf("empty identity group")
		default:
			continue
		}
		if _, exists := seen[role]; exists {
			return nil, fmt.Errorf("duplicate identity group")
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles, nil
}

func RequireRecentAuthentication(principal Principal, now time.Time, maximumAge time.Duration) error {
	if maximumAge <= 0 || !principal.Valid(now) || now.Sub(principal.AuthenticatedAt) > maximumAge {
		return fmt.Errorf("recent authentication required")
	}
	return nil
}
