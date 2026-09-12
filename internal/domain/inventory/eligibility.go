package inventory

// IsEligibleForNewCampaignWork intentionally reflects only administrative
// availability. Connectivity, compatibility, capacity, and maintenance-window
// decisions are independent policies owned by their respective domain slices.
func (state AdministrativeState) IsEligibleForNewCampaignWork() bool {
	return state == AdministrativeStateActive
}
