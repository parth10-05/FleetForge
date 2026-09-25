// Package model is an independent test-only lifecycle oracle. It deliberately
// duplicates the public state-machine contract without importing core.
package model

import "testing"

type state string

const (
	accepted             state = "accepted"
	running              state = "running"
	rebooting            state = "rebooting"
	inventoryPending     state = "inventory-pending"
	completed            state = "completed"
	healthy              state = "healthy"
	taskMissing          state = "task-missing"
	versionContradiction state = "version-contradiction"
	healthFailed         state = "health-failed"
)

func next(from state, fault string) (state, bool) {
	switch from {
	case accepted:
		return running, true
	case running:
		return rebooting, true
	case rebooting:
		if fault == "RF-F08" {
			return taskMissing, true
		}
		return inventoryPending, true
	case inventoryPending, taskMissing:
		if fault == "RF-F10" {
			return versionContradiction, true
		}
		if fault == "RF-F11" {
			return healthFailed, true
		}
		return completed, true
	case completed:
		return healthy, true
	}
	return "", false
}

func TestDirectedLifecycleCoverage(t *testing.T) {
	coverage := map[state]int{}
	for _, fault := range []string{"", "RF-F08", "RF-F10", "RF-F11"} {
		current := accepted
		coverage[current]++
		for steps := 0; steps < 5; steps++ {
			useFault := ""
			if current == rebooting || current == inventoryPending || current == taskMissing {
				useFault = fault
			}
			nextState, ok := next(current, useFault)
			if !ok {
				break
			}
			current = nextState
			coverage[current]++
			if current == healthy || current == versionContradiction || current == healthFailed {
				break
			}
		}
	}
	for _, required := range []state{accepted, running, rebooting, inventoryPending, completed, healthy, taskMissing, versionContradiction, healthFailed} {
		if coverage[required] == 0 {
			t.Errorf("directed prefixes did not cover %s", required)
		}
	}
	if _, ok := next(healthy, ""); ok {
		t.Fatal("terminal healthy state has an illegal transition")
	}
}
