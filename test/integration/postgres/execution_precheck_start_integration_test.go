//go:build postgresintegration

package postgres

import (
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestCanaryStartSchedulesFencedPrecheckWithoutAdmission(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareAdmissionFixture(t, pool, ctx, fixture, 7)
	const workID = "00000000-0000-0000-5301-000000000001"
	const commandID = "00000000-0000-0000-5301-000000000002"
	const lease = "00000000-0000-4000-8000-000000000003"
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch,delivery_state,lease_token,lease_owner,lease_expires_at)
VALUES ($1::uuid,$2::uuid,'TARGET_UPDATE',$3::uuid,'CANARY_START',1,$4,0,7,'CLAIMED',$5::uuid,'start-test',$6)`, workID, commandID, fixture.targetUpdateID, now.Add(-time.Second), lease, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	epoch := int64(7)
	work := execution.WorkFence{DueWorkID: workID, CommandID: commandID, LeaseToken: lease, WorkType: "CANARY_START", ExpectedRevision: 0, ExpectedCommandEpoch: &epoch}
	if err := persistence.StartFirmwarePrecheck(ctx, pool, work, now); err != nil {
		t.Fatal(err)
	}
	var state, startDelivery, nextDelivery, nextType string
	var revision, nextRevision int64
	if err := pool.QueryRow(ctx, `SELECT tu.state,tu.revision,old.delivery_state,next.delivery_state,next.work_type,next.expected_revision
FROM target_updates tu JOIN due_work old ON old.id=$2::uuid JOIN due_work next ON next.subject_id=tu.id AND next.work_type='PRECHECK'
WHERE tu.id=$1::uuid`, fixture.targetUpdateID, workID).Scan(&state, &revision, &startDelivery, &nextDelivery, &nextType, &nextRevision); err != nil {
		t.Fatal(err)
	}
	if state != "PRECHECK" || revision != 1 || startDelivery != "COMPLETED" || nextDelivery != "PENDING" || nextType != "PRECHECK" || nextRevision != 1 {
		t.Fatalf("state=%s/%d start=%s next=%s/%s/%d", state, revision, startDelivery, nextDelivery, nextType, nextRevision)
	}
	for name, query := range map[string]string{
		"decision": "SELECT COUNT(*) FROM decision_inputs WHERE aggregate_id='" + fixture.targetUpdateID + "'::uuid AND result='PRECHECK'",
		"audit":    "SELECT COUNT(*) FROM audit_events WHERE aggregate_id='" + fixture.targetUpdateID + "'::uuid AND action='FIRMWARE_PRECHECK_STARTED'",
		"outbox":   "SELECT COUNT(*) FROM outbox_events WHERE aggregate_id='" + fixture.targetUpdateID + "'::uuid AND event_type='target.precheck_started'",
		"attempt":  "SELECT COUNT(*) FROM firmware_update_attempts WHERE target_update_id='" + fixture.targetUpdateID + "'::uuid",
	} {
		var count int
		if err := pool.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Fatalf("%s query: %v", name, err)
		}
		want := 1
		if name == "attempt" {
			want = 0
		}
		if count != want {
			t.Fatalf("%s count=%d want=%d", name, count, want)
		}
	}
}
