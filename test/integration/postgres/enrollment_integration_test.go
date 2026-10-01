//go:build postgresintegration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

// ─── P03-09: readiness enrollment transaction integration proofs ─────────────

// TestReadinessEnrollmentTransitionsCampaignToReady proves the happy path:
// a single target enrolled successfully transitions the campaign to READY
// and creates both an audit event and an outbox event atomically.
func TestReadinessEnrollmentTransitionsCampaignToReady(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareReadinessCampaign(t, pool, ctx, fixture)

	req := enrollmentRequest(fixture, "00000000-0000-0000-9001-000000000001", "00000000-0000-0000-9001-000000000002")
	if err := persistence.AcquireReadinessEnrollment(ctx, pool, req); err != nil {
		t.Fatalf("AcquireReadinessEnrollment: %v", err)
	}

	var state string
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT state, revision FROM campaigns WHERE id=$1", fixture.campaignID).Scan(&state, &revision); err != nil {
		t.Fatalf("read campaign: %v", err)
	}
	if state != "READY" || revision != 1 {
		t.Errorf("campaign after readiness: state=%q revision=%d, want READY/1", state, revision)
	}

	var enrollmentState string
	if err := pool.QueryRow(ctx, "SELECT state FROM target_enrollments WHERE target_id=$1", fixture.targetID).Scan(&enrollmentState); err != nil {
		t.Fatalf("read enrollment: %v", err)
	}
	if enrollmentState != "ACTIVE" {
		t.Errorf("enrollment state = %q, want ACTIVE", enrollmentState)
	}

	var auditCount, outboxCount int
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid", fixture.campaignID).Scan(&auditCount)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid", fixture.campaignID).Scan(&outboxCount)
	if auditCount != 1 || outboxCount != 1 {
		t.Errorf("audit=%d outbox=%d, want 1/1", auditCount, outboxCount)
	}
}

// TestReadinessEnrollmentBlockedByExistingActiveEnrollment proves that when a
// target already has an active enrollment, AcquireReadinessEnrollment returns
// ErrEnrollmentBlocked and the campaign remains unchanged.
func TestReadinessEnrollmentBlockedByExistingActiveEnrollment(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareReadinessCampaign(t, pool, ctx, fixture)
	// Pre-insert an active enrollment for the target under a different campaign context.
	insertEnrollment(t, pool, ctx, "00000000-0000-0000-9002-000000000001", fixture)

	req := enrollmentRequest(fixture, "00000000-0000-0000-9002-000000000010", "00000000-0000-0000-9002-000000000011")
	err := persistence.AcquireReadinessEnrollment(ctx, pool, req)
	if !errors.Is(err, persistence.ErrEnrollmentBlocked) {
		t.Errorf("already-enrolled target error = %v, want ErrEnrollmentBlocked", err)
	}

	// Campaign must remain unchanged (VALIDATING, revision=0).
	var state string
	var revision int64
	_ = pool.QueryRow(ctx, "SELECT state, revision FROM campaigns WHERE id=$1", fixture.campaignID).Scan(&state, &revision)
	if state != "VALIDATING" || revision != 0 {
		t.Errorf("campaign after blocked enrollment: state=%q revision=%d, want VALIDATING/0", state, revision)
	}
}

// TestReadinessEnrollmentBlockedByUnresolvedReservation proves that neither a
// live nor quarantined physical-operation reservation can be bypassed by a
// later readiness transaction.
func TestReadinessEnrollmentBlockedByUnresolvedReservation(t *testing.T) {
	for _, reservationState := range []string{"ACTIVE", "QUARANTINED"} {
		t.Run(reservationState, func(t *testing.T) {
			_, pool, ctx := migratedHarness(t)
			fixture := seedOwnershipFixture(t, pool, ctx)
			prepareReadinessCampaign(t, pool, ctx, fixture)
			admissionID := "00000000-0000-0000-9003-000000000001"
			insertAdmission(t, pool, ctx, admissionID, "00000000-0000-0000-9003-000000000002", fixture)
			if _, err := pool.Exec(ctx,
				"INSERT INTO operation_reservations (id, admission_id, target_id, campaign_id, scope_level, scope_key, state) "+
					"VALUES ('00000000-0000-0000-9003-000000000010', $1, $2, $3, 'GLOBAL', 'GLOBAL', $4)",
				admissionID, fixture.targetID, fixture.campaignID, reservationState,
			); err != nil {
				t.Fatalf("insert %s reservation: %v", reservationState, err)
			}
			req := enrollmentRequest(fixture, "00000000-0000-0000-9003-000000000020", "00000000-0000-0000-9003-000000000021")
			if err := persistence.AcquireReadinessEnrollment(ctx, pool, req); !errors.Is(err, persistence.ErrEnrollmentBlocked) {
				t.Errorf("%s target error = %v, want ErrEnrollmentBlocked", reservationState, err)
			}
		})
	}
}

// TestReadinessEnrollmentRollsBackEarlierClaimsWhenALaterMemberIsBlocked
// proves that a multi-member readiness handoff remains all-or-nothing. The
// first target is eligible, but the second already has an active enrollment;
// neither target may be enrolled and no campaign/audit/outbox state may be
// committed.
func TestReadinessEnrollmentRollsBackEarlierClaimsWhenALaterMemberIsBlocked(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	second := seedSecondTargetForReadiness(t, pool, ctx, fixture)
	prepareReadinessCampaign(t, pool, ctx, fixture)
	blocker := prepareReadinessCandidateTarget(t, pool, ctx, fixture, second)

	// The candidate and campaign own both targets, but an earlier campaign has
	// retained the later target. The first target must not be left enrolled when
	// the later target blocks the ordered transaction.
	insertEnrollment(t, pool, ctx, "00000000-0000-0000-9007-000000000001", blocker)
	req := enrollmentRequest(fixture, "00000000-0000-0000-9007-000000000002", "00000000-0000-0000-9007-000000000003")
	req.Targets = append(req.Targets, persistence.TargetEnrollmentEntry{
		EnrollmentID:     "00000000-0000-0000-9007-000000000004",
		TargetID:         second.targetID,
		CampaignTargetID: second.campaignTargetID,
	})
	if err := persistence.AcquireReadinessEnrollment(ctx, pool, req); !errors.Is(err, persistence.ErrEnrollmentBlocked) {
		t.Fatalf("AcquireReadinessEnrollment error = %v, want ErrEnrollmentBlocked", err)
	}

	var state string
	var revision, enrollments, audits, outbox int
	if err := pool.QueryRow(ctx, "SELECT state,revision FROM campaigns WHERE id=$1", fixture.campaignID).Scan(&state, &revision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM target_enrollments WHERE campaign_id=$1", fixture.campaignID).Scan(&enrollments); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events WHERE aggregate_id=$1::uuid", fixture.campaignID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE aggregate_id=$1::uuid", fixture.campaignID).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if state != "VALIDATING" || revision != 0 || enrollments != 0 || audits != 0 || outbox != 0 {
		t.Fatalf("blocked multi-member readiness wrote durable state: campaign=%s/%d enrollments=%d audits=%d outbox=%d", state, revision, enrollments, audits, outbox)
	}
}

// TestReadinessEnrollmentCompetingCampaignsOnlyOneWins proves that when two
// campaigns compete to enroll the same target, exactly one succeeds and the
// other receives ErrEnrollmentBlocked or ErrRevisionConflict.
func TestReadinessEnrollmentCompetingCampaignsOnlyOneWins(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	second := seedSecondCampaignTarget(t, pool, ctx, fixture)
	prepareReadinessCampaign(t, pool, ctx, fixture)
	prepareReadinessCampaign(t, pool, ctx, second)

	type result struct{ err error }
	results := make(chan result, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		req := enrollmentRequest(fixture, "00000000-0000-0000-9004-000000000001", "00000000-0000-0000-9004-000000000002")
		results <- result{err: persistence.AcquireReadinessEnrollment(context.Background(), pool, req)}
	}()
	go func() {
		defer wg.Done()
		secondFixture := ownershipFixture{
			targetID: fixture.targetID, campaignID: second.campaignID,
			campaignTargetID: second.campaignTargetID, targetUpdateID: fixture.targetUpdateID,
			artifactID: fixture.artifactID, admissionID: fixture.admissionID,
		}
		req := enrollmentRequest(secondFixture, "00000000-0000-0000-9004-000000000010", "00000000-0000-0000-9004-000000000011")
		results <- result{err: persistence.AcquireReadinessEnrollment(context.Background(), pool, req)}
	}()
	wg.Wait()
	close(results)

	var successes, blocked int
	for r := range results {
		if r.err == nil {
			successes++
		} else if errors.Is(r.err, persistence.ErrEnrollmentBlocked) || errors.Is(r.err, persistence.ErrRevisionConflict) {
			blocked++
		} else {
			t.Errorf("unexpected error: %v", r.err)
		}
	}
	if successes != 1 || blocked != 1 {
		t.Errorf("competing campaigns: successes=%d blocked=%d, want 1/1", successes, blocked)
	}
}

// TestReadinessEnrollmentTerminalTargetUpdateDoesNotReleaseEnrollment proves
// that marking a target_update as SUCCEEDED does not remove the active
// enrollment (TTL or terminality cannot release a safety claim).
func TestReadinessEnrollmentTerminalTargetUpdateDoesNotReleaseEnrollment(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareReadinessCampaign(t, pool, ctx, fixture)

	req := enrollmentRequest(fixture, "00000000-0000-0000-9005-000000000001", "00000000-0000-0000-9005-000000000002")
	if err := persistence.AcquireReadinessEnrollment(ctx, pool, req); err != nil {
		t.Fatalf("AcquireReadinessEnrollment: %v", err)
	}

	// Mark target_update SUCCEEDED (terminal).
	if _, err := pool.Exec(ctx, "UPDATE target_updates SET state='SUCCEEDED' WHERE id=$1", fixture.targetUpdateID); err != nil {
		t.Fatalf("mark terminal: %v", err)
	}

	var enrollmentState string
	if err := pool.QueryRow(ctx, "SELECT state FROM target_enrollments WHERE target_id=$1", fixture.targetID).Scan(&enrollmentState); err != nil {
		t.Fatalf("read enrollment: %v", err)
	}
	if enrollmentState != "ACTIVE" {
		t.Errorf("enrollment after terminal target update = %q, want ACTIVE", enrollmentState)
	}
}

// TestReadinessEnrollmentRejectsInvalidatedTrustInputsAtomically proves that a
// candidate cannot be used after its artifact is retired, its preflight result
// is blocked, or its target revision has drifted. Every rejection occurs before
// an enrollment, campaign transition, audit event, or outbox event is written.
func TestReadinessEnrollmentRejectsInvalidatedTrustInputsAtomically(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*pgxpool.Pool, context.Context, *ownershipFixture) error
	}{
		{
			name: "retired artifact",
			mutate: func(pool *pgxpool.Pool, ctx context.Context, fixture *ownershipFixture) error {
				return persistence.RetireVerifiedArtifact(ctx, pool, fixture.artifactID)
			},
		},
		{
			name: "unknown candidate",
			mutate: func(_ *pgxpool.Pool, _ context.Context, fixture *ownershipFixture) error {
				fixture.candidateID = "00000000-0000-0000-9006-000000000099"
				return nil
			},
		},
		{
			name: "drifted target revision",
			mutate: func(pool *pgxpool.Pool, ctx context.Context, fixture *ownershipFixture) error {
				_, err := pool.Exec(ctx, "UPDATE targets SET revision=revision+1 WHERE id=$1", fixture.targetID)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, pool, ctx := migratedHarness(t)
			fixture := seedOwnershipFixture(t, pool, ctx)
			prepareReadinessCampaign(t, pool, ctx, fixture)
			if err := test.mutate(pool, ctx, &fixture); err != nil {
				t.Fatalf("invalidate trust input: %v", err)
			}

			err := persistence.AcquireReadinessEnrollment(ctx, pool, enrollmentRequest(fixture, "00000000-0000-0000-9006-000000000001", "00000000-0000-0000-9006-000000000002"))
			if !errors.Is(err, persistence.ErrEnrollmentBlocked) {
				t.Fatalf("AcquireReadinessEnrollment error = %v, want ErrEnrollmentBlocked", err)
			}

			var campaignState string
			var campaignRevision, enrollments, audits, outbox int
			if err := pool.QueryRow(ctx, "SELECT state,revision FROM campaigns WHERE id=$1", fixture.campaignID).Scan(&campaignState, &campaignRevision); err != nil {
				t.Fatalf("read campaign after blocked readiness: %v", err)
			}
			if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM target_enrollments WHERE campaign_id=$1", fixture.campaignID).Scan(&enrollments); err != nil {
				t.Fatalf("count enrollments after blocked readiness: %v", err)
			}
			if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events WHERE aggregate_id=$1::uuid", fixture.campaignID).Scan(&audits); err != nil {
				t.Fatalf("count audit after blocked readiness: %v", err)
			}
			if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE aggregate_id=$1::uuid", fixture.campaignID).Scan(&outbox); err != nil {
				t.Fatalf("count outbox after blocked readiness: %v", err)
			}
			if campaignState != "VALIDATING" || campaignRevision != 0 || enrollments != 0 || audits != 0 || outbox != 0 {
				t.Fatalf("blocked readiness wrote durable state: campaign=%s/%d enrollments=%d audits=%d outbox=%d", campaignState, campaignRevision, enrollments, audits, outbox)
			}
		})
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func enrollmentRequest(fixture ownershipFixture, enrollmentID, outboxEventID string) persistence.ReadinessEnrollmentRequest {
	return persistence.ReadinessEnrollmentRequest{
		CampaignID:               fixture.campaignID,
		CandidateID:              fixture.candidateID,
		PolicyVersionID:          "00000000-0000-0000-0000-000000000207",
		ExpectedRevision:         0,
		FrozenPolicyHash:         "policy-hash",
		FrozenTargetSnapshotHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Targets: []persistence.TargetEnrollmentEntry{{
			EnrollmentID:     enrollmentID,
			TargetID:         fixture.targetID,
			CampaignTargetID: fixture.campaignTargetID,
		}},
		ActorID:             "system",
		CorrelationID:       "00000000-0000-0000-9000-000000000001",
		CausationID:         "00000000-0000-0000-9000-000000000001",
		Action:              "READINESS_ACQUIRED",
		ReasonCode:          "campaign.validation.accepted",
		EventSequence:       0,
		OutboxEventID:       outboxEventID,
		OutboxEventType:     "campaign.validation_succeeded",
		OutboxSchemaVersion: 1,
		OutboxPayload:       json.RawMessage(`{"version":"v1"}`),
	}
}

func prepareReadinessCampaign(t *testing.T, pool *pgxpool.Pool, ctx context.Context, fixture ownershipFixture) {
	t.Helper()
	if _, err := pool.Exec(ctx, "UPDATE campaigns SET state='VALIDATING' WHERE id=$1::uuid", fixture.campaignID); err != nil {
		t.Fatalf("prepare campaign for readiness: %v", err)
	}
	// P02 supplies the immutable trust input for the P03 readiness handoff.
	// The records deliberately mirror the candidate-time checks: exact object
	// version, verified artifact, trusted target identity, and fresh discovery.
	mustExecute(t, pool, ctx, "UPDATE targets SET target_key='target-race-key', contract_version='synthetic-redfish-v1', credential_provider='synthetic-test', credential_reference='synthetic-test:target-race', transport_policy_version='transport-v1', registration_hash='sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', trust_metadata_complete=true WHERE id='"+fixture.targetID+"'")
	mustExecute(t, pool, ctx, "INSERT INTO target_labels (target_id,key,value) VALUES ('"+fixture.targetID+"','family','synthetic-family') ON CONFLICT (target_id,key) DO UPDATE SET value=EXCLUDED.value")
	mustExecute(t, pool, ctx, "INSERT INTO device_observations (id,target_id,kind,observed_at,normalized_data) VALUES ('00000000-0000-0000-0000-000000000214','"+fixture.targetID+"','DISCOVERY',now(),'{}') ON CONFLICT (id) DO NOTHING")
	mustExecute(t, pool, ctx, "INSERT INTO target_discovery_heads (target_id,head_revision,latest_attempt_id,last_good_id) SELECT id,revision,'00000000-0000-0000-0000-000000000214','00000000-0000-0000-0000-000000000214' FROM targets WHERE id='"+fixture.targetID+"' ON CONFLICT (target_id) DO UPDATE SET head_revision=EXCLUDED.head_revision,latest_attempt_id=EXCLUDED.latest_attempt_id,last_good_id=EXCLUDED.last_good_id")
	mustExecute(t, pool, ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ('"+fixture.artifactID+"','local-versioned-v1','fixture','firmware-v1','version-1',1,'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','synthetic-sha256-v1','synthetic-family') ON CONFLICT (artifact_id) DO NOTHING")
	mustExecute(t, pool, ctx, "INSERT INTO preflight_candidates (id,artifact_id,artifact_version_id,selector_version,selector_json,snapshot_hash,state,report) VALUES ('"+fixture.candidateID+"','"+fixture.artifactID+"','version-1','selector-v1','{}','sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','PASSED','{}')")
	mustExecute(t, pool, ctx, "INSERT INTO preflight_candidate_targets (candidate_id,target_id,frozen_target_revision,site_key,rack_key,target_family,compatibility_policy_version,compatibility_reason) SELECT '"+fixture.candidateID+"','"+fixture.targetID+"',revision,'site-a','rack-a','synthetic-family','synthetic-family-exact-v1','compatible' FROM targets WHERE id='"+fixture.targetID+"'")
}

func seedSecondTargetForReadiness(t *testing.T, pool *pgxpool.Pool, ctx context.Context, fixture ownershipFixture) ownershipFixture {
	t.Helper()
	second := fixture
	second.targetID = "00000000-0000-0000-0000-000000000230"
	second.campaignTargetID = "00000000-0000-0000-0000-000000000231"
	second.targetUpdateID = "00000000-0000-0000-0000-000000000232"
	mustExecute(t, pool, ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state,target_key,contract_version,credential_provider,credential_reference,transport_policy_version,registration_hash,trust_metadata_complete) VALUES ('"+second.targetID+"','target-second','ref','site-a','rack-b','ACTIVE','target-second-key','synthetic-redfish-v1','synthetic-test','synthetic-test:target-second','transport-v1','sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd',true)")
	mustExecute(t, pool, ctx, "INSERT INTO campaign_targets (id,campaign_id,target_id,artifact_id,frozen_target_revision,frozen_policy_hash) VALUES ('"+second.campaignTargetID+"','"+fixture.campaignID+"','"+second.targetID+"','"+fixture.artifactID+"',0,'policy-hash')")
	mustExecute(t, pool, ctx, "INSERT INTO target_updates (id,campaign_target_id,state,expected_firmware_version) VALUES ('"+second.targetUpdateID+"','"+second.campaignTargetID+"','PENDING','v1')")
	return second
}

func prepareReadinessCandidateTarget(t *testing.T, pool *pgxpool.Pool, ctx context.Context, fixture, second ownershipFixture) ownershipFixture {
	t.Helper()
	mustExecute(t, pool, ctx, "INSERT INTO target_labels (target_id,key,value) VALUES ('"+second.targetID+"','family','synthetic-family')")
	mustExecute(t, pool, ctx, "INSERT INTO device_observations (id,target_id,kind,observed_at,normalized_data) VALUES ('00000000-0000-0000-0000-000000000233','"+second.targetID+"','DISCOVERY',now(),'{}')")
	mustExecute(t, pool, ctx, "INSERT INTO target_discovery_heads (target_id,head_revision,latest_attempt_id,last_good_id) SELECT id,revision,'00000000-0000-0000-0000-000000000233','00000000-0000-0000-0000-000000000233' FROM targets WHERE id='"+second.targetID+"'")
	mustExecute(t, pool, ctx, "INSERT INTO preflight_candidate_targets (candidate_id,target_id,frozen_target_revision,site_key,rack_key,target_family,compatibility_policy_version,compatibility_reason) SELECT '"+fixture.candidateID+"','"+second.targetID+"',revision,'site-a','rack-b','synthetic-family','synthetic-family-exact-v1','compatible' FROM targets WHERE id='"+second.targetID+"'")

	// Create a distinct historical campaign solely to retain the active claim
	// that blocks this target. It deliberately does not share the candidate.
	mustExecute(t, pool, ctx, "INSERT INTO campaigns (id,artifact_id,state,policy_version_id) VALUES ('00000000-0000-0000-0000-000000000234','"+fixture.artifactID+"','DRAFT','00000000-0000-0000-0000-000000000207')")
	mustExecute(t, pool, ctx, "INSERT INTO campaign_targets (id,campaign_id,target_id,artifact_id,frozen_target_revision,frozen_policy_hash) VALUES ('00000000-0000-0000-0000-000000000235','00000000-0000-0000-0000-000000000234','"+second.targetID+"','"+fixture.artifactID+"',0,'policy-hash')")
	blocker := second
	blocker.campaignID = "00000000-0000-0000-0000-000000000234"
	blocker.campaignTargetID = "00000000-0000-0000-0000-000000000235"
	return blocker
}
