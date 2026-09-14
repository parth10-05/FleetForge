//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestOwnershipSchemaAllowsOnlyOneActiveEnrollmentDuringRace(t *testing.T) {
	harness, pool, ctx := migratedHarness(t)
	_ = harness
	fixture := seedOwnershipFixture(t, pool, ctx)
	secondFixture := seedSecondCampaignTarget(t, pool, ctx, fixture)
	first, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first enrollment transaction: %v", err)
	}
	defer first.Rollback(ctx)
	insertEnrollment(t, first, ctx, "00000000-0000-0000-0000-000000000101", fixture)

	secondDone := make(chan error, 1)
	go func() {
		transaction, err := pool.Begin(context.Background())
		if err != nil {
			secondDone <- err
			return
		}
		defer transaction.Rollback(context.Background())
		_, err = transaction.Exec(context.Background(), enrollmentStatement("00000000-0000-0000-0000-000000000102", secondFixture))
		if err == nil {
			err = transaction.Commit(context.Background())
		}
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("second enrollment completed before first commit: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatalf("commit first enrollment: %v", err)
	}
	if err := <-secondDone; !isUniqueViolation(err) {
		t.Fatalf("second active enrollment error = %v, want unique violation", err)
	}
}

func TestAdmissionIsPermanentAndReservationSurvivesTargetTerminality(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	insertEnrollment(t, pool, ctx, "00000000-0000-0000-0000-000000000103", fixture)
	insertAdmission(t, pool, ctx, "00000000-0000-0000-0000-000000000104", "00000000-0000-0000-0000-000000000105", fixture)
	_, err := pool.Exec(ctx, admissionStatement("00000000-0000-0000-0000-000000000106", "00000000-0000-0000-0000-000000000107", fixture))
	if !isUniqueViolation(err) {
		t.Fatalf("duplicate submission admission error = %v, want unique violation", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO operation_reservations (id, admission_id, target_id, campaign_id, scope_level, scope_key, state) VALUES ('00000000-0000-0000-0000-000000000108', $1, $2, $3, 'GLOBAL', 'GLOBAL', 'ACTIVE')", fixture.admissionID, fixture.targetID, fixture.campaignID)
	if err != nil {
		t.Fatalf("insert durable operation reservation: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO operation_reservations (id, admission_id, target_id, campaign_id, scope_level, scope_key, state) VALUES ('00000000-0000-0000-0000-000000000109', $1, $2, $3, 'GLOBAL', 'GLOBAL', 'ACTIVE')", fixture.admissionID, fixture.targetID, fixture.campaignID)
	if !isUniqueViolation(err) {
		t.Fatalf("duplicate active operation reservation error = %v, want unique violation", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE target_updates SET state = 'SUCCEEDED' WHERE id = $1", fixture.targetUpdateID); err != nil {
		t.Fatalf("mark target update terminal: %v", err)
	}
	var reservationState, enrollmentState string
	if err := pool.QueryRow(ctx, "SELECT state FROM operation_reservations WHERE id = '00000000-0000-0000-0000-000000000108'").Scan(&reservationState); err != nil {
		t.Fatalf("load operation reservation: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT state FROM target_enrollments WHERE target_id = $1", fixture.targetID).Scan(&enrollmentState); err != nil {
		t.Fatalf("load target enrollment: %v", err)
	}
	if reservationState != "ACTIVE" || enrollmentState != "ACTIVE" {
		t.Fatalf("terminal target update changed safety claims: reservation=%q enrollment=%q", reservationState, enrollmentState)
	}
}

type ownershipFixture struct {
	targetID, artifactID, campaignID, campaignTargetID, targetUpdateID, admissionID string
}

func migratedHarness(t *testing.T) (Harness, *pgxpool.Pool, context.Context) {
	t.Helper()
	harness := New(t)
	if err := persistence.RunMigrations(context.Background(), integrationSettings(harness.ScopedDSN(t)), os.DirFS(filepath.Join("..", "..", "..", "db", "migrations"))); err != nil {
		t.Fatalf("migrate ownership schema: %v", err)
	}
	return harness, harness.OpenPool(t), context.Background()
}

func seedOwnershipFixture(t *testing.T, pool *pgxpool.Pool, ctx context.Context) ownershipFixture {
	t.Helper()
	fixture := ownershipFixture{
		targetID: "00000000-0000-0000-0000-000000000201", artifactID: "00000000-0000-0000-0000-000000000202", campaignID: "00000000-0000-0000-0000-000000000203",
		campaignTargetID: "00000000-0000-0000-0000-000000000204", targetUpdateID: "00000000-0000-0000-0000-000000000205", admissionID: "00000000-0000-0000-0000-000000000206",
	}
	mustExecute(t, pool, ctx, fmt.Sprintf("INSERT INTO targets (id, external_identity, endpoint_reference, site_key, rack_key, administrative_state) VALUES ('%s', 'target-race', 'ref', 'site-a', 'rack-a', 'ACTIVE')", fixture.targetID))
	mustExecute(t, pool, ctx, fmt.Sprintf("INSERT INTO firmware_artifacts (id, firmware_version, sha256_digest, object_reference, state) VALUES ('%s', 'v1', 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'ref', 'VERIFIED')", fixture.artifactID))
	mustExecute(t, pool, ctx, fmt.Sprintf("INSERT INTO campaigns (id, artifact_id, state, policy_version_id) VALUES ('%s', '%s', 'DRAFT', '00000000-0000-0000-0000-000000000207')", fixture.campaignID, fixture.artifactID))
	mustExecute(t, pool, ctx, fmt.Sprintf("INSERT INTO campaign_targets (id, campaign_id, target_id, artifact_id, frozen_target_revision, frozen_policy_hash) VALUES ('%s', '%s', '%s', '%s', 0, 'policy-hash')", fixture.campaignTargetID, fixture.campaignID, fixture.targetID, fixture.artifactID))
	mustExecute(t, pool, ctx, fmt.Sprintf("INSERT INTO target_updates (id, campaign_target_id, state, expected_firmware_version) VALUES ('%s', '%s', 'PENDING', 'v1')", fixture.targetUpdateID, fixture.campaignTargetID))
	return fixture
}

func seedSecondCampaignTarget(t *testing.T, pool *pgxpool.Pool, ctx context.Context, original ownershipFixture) ownershipFixture {
	t.Helper()
	second := original
	second.campaignID = "00000000-0000-0000-0000-000000000210"
	second.campaignTargetID = "00000000-0000-0000-0000-000000000211"
	mustExecute(t, pool, ctx, fmt.Sprintf("INSERT INTO campaigns (id, artifact_id, state, policy_version_id) VALUES ('%s', '%s', 'DRAFT', '00000000-0000-0000-0000-000000000212')", second.campaignID, second.artifactID))
	mustExecute(t, pool, ctx, fmt.Sprintf("INSERT INTO campaign_targets (id, campaign_id, target_id, artifact_id, frozen_target_revision, frozen_policy_hash) VALUES ('%s', '%s', '%s', '%s', 0, 'policy-hash')", second.campaignTargetID, second.campaignID, second.targetID, second.artifactID))
	return second
}

func insertEnrollment(t *testing.T, executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, ctx context.Context, id string, fixture ownershipFixture) {
	t.Helper()
	mustExecute(t, executor, ctx, enrollmentStatement(id, fixture))
}

func enrollmentStatement(id string, fixture ownershipFixture) string {
	return fmt.Sprintf("INSERT INTO target_enrollments (id, target_id, campaign_id, campaign_target_id) VALUES ('%s', '%s', '%s', '%s')", id, fixture.targetID, fixture.campaignID, fixture.campaignTargetID)
}

func insertAdmission(t *testing.T, executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, ctx context.Context, id, commandID string, fixture ownershipFixture) {
	t.Helper()
	mustExecute(t, executor, ctx, admissionStatement(id, commandID, fixture))
}

func admissionStatement(id, commandID string, fixture ownershipFixture) string {
	return fmt.Sprintf("INSERT INTO submission_admissions (id, campaign_id, target_id, artifact_id, target_update_id, command_id, step, captured_command_epoch) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', 'SUBMISSION', 0)", id, fixture.campaignID, fixture.targetID, fixture.artifactID, fixture.targetUpdateID, commandID)
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
