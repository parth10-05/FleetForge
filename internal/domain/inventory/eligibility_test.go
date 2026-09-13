package inventory

import "testing"

func TestOnlyAdministrativelyActiveTargetIsEligible(t *testing.T) {
	if !AdministrativeStateActive.IsEligibleForNewCampaignWork() {
		t.Fatal("active target must be administratively eligible")
	}
	for _, state := range []AdministrativeState{AdministrativeStateDisabled, AdministrativeStateUnreachable, AdministrativeStateMaintenanceBlocked, "UNKNOWN"} {
		if state.IsEligibleForNewCampaignWork() {
			t.Fatalf("%s must not be administratively eligible", state)
		}
	}
}
