package access

import (
	"fmt"
	"time"
)

type Permission string

const (
	PermissionReadTargets       Permission = "read_targets"
	PermissionReadArtifacts     Permission = "read_artifacts"
	PermissionRegisterTargets   Permission = "register_targets"
	PermissionDiscoverTargets   Permission = "discover_targets"
	PermissionInitiateArtifacts Permission = "initiate_artifacts"
	PermissionCreateCampaigns   Permission = "create_campaigns"
	PermissionOperateCampaigns  Permission = "operate_campaigns"
	PermissionResolveAttention  Permission = "resolve_attention"
)

func Authorize(principal Principal, permission Permission, now time.Time) error {
	if !principal.Valid(now) {
		return fmt.Errorf("unauthenticated principal")
	}
	if permitted(principal, permission) {
		return nil
	}
	return fmt.Errorf("permission denied")
}

func permitted(principal Principal, permission Permission) bool {
	if principal.HasRole(RoleAdmin) {
		return true
	}
	switch permission {
	case PermissionReadTargets, PermissionReadArtifacts:
		return principal.HasRole(RoleViewer) || principal.HasRole(RoleOperator)
	case PermissionCreateCampaigns, PermissionOperateCampaigns:
		return principal.HasRole(RoleOperator)
	default:
		return false
	}
}
