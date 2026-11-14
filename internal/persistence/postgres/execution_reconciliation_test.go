package postgres

import (
	"testing"

	"github.com/parth10-05/fleetforge/internal/application/execution"
)

func TestAdoptionSchedulesInventoryVerificationWithoutTaskReference(t *testing.T) {
	state, status, work, _ := reconciliationTransition("ADOPT")
	if state != "VERIFYING" || status != "ADOPTED" || work != "INVENTORY_OBSERVE" {
		t.Fatalf("reconciliation adoption=%s/%s/%s", state, status, work)
	}
	_, state, status, work, _, err := adminResolutionTransition(execution.AdminResolutionCommand{Action: "ADOPT"})
	if err != nil || state != "VERIFYING" || status != "ADOPTED" || work != "INVENTORY_OBSERVE" {
		t.Fatalf("administrator adoption=%s/%s/%s err=%v", state, status, work, err)
	}
}
