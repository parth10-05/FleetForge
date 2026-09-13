package inventory

import "errors"

var ErrUnknownAdministrativeState = errors.New("unknown target administrative state")

// AdministrativeState describes administrative eligibility independently from
// a campaign-specific target-update lifecycle or transient probe result.
type AdministrativeState string

const (
	AdministrativeStateActive             AdministrativeState = "ACTIVE"
	AdministrativeStateDisabled           AdministrativeState = "DISABLED"
	AdministrativeStateUnreachable        AdministrativeState = "UNREACHABLE"
	AdministrativeStateMaintenanceBlocked AdministrativeState = "MAINTENANCE_BLOCKED"
)

var allAdministrativeStates = []AdministrativeState{
	AdministrativeStateActive,
	AdministrativeStateDisabled,
	AdministrativeStateUnreachable,
	AdministrativeStateMaintenanceBlocked,
}

func ParseAdministrativeState(raw string) (AdministrativeState, error) {
	state := AdministrativeState(raw)
	for _, candidate := range allAdministrativeStates {
		if state == candidate {
			return state, nil
		}
	}
	return "", ErrUnknownAdministrativeState
}

func (state AdministrativeState) String() string { return string(state) }

func AllAdministrativeStates() []AdministrativeState {
	return append([]AdministrativeState(nil), allAdministrativeStates...)
}
