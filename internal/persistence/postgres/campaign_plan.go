package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var ErrCampaignPlanConflict = errors.New("campaign plan conflicts with current draft")

// CampaignPlanMember supplies generated durable identities for a planner input.
// It intentionally carries no policy fact: those come only from the immutable
// P02 candidate and the canonical domain plan.
type CampaignPlanMember struct {
	TargetID         string
	CampaignTargetID string
	TargetUpdateID   string
}

type CampaignPlanPersistenceRequest struct {
	CampaignID       string
	CandidateID      string
	PolicyVersionID  string
	ExpectedRevision int64
	CanaryWaveID     string
	LaterWaveID      string
	Members          []CampaignPlanMember
	Plan             campaignplan.Plan
}

// PersistCampaignPlan transitions a DRAFT campaign into VALIDATING and stores
// the complete immutable candidate-backed plan. It performs no enrollment,
// outbox publication, worker claim, or external I/O; READY remains exclusively
// owned by AcquireReadinessEnrollment after its P02 trust recheck.
func PersistCampaignPlan(ctx context.Context, pool *pgxpool.Pool, request CampaignPlanPersistenceRequest) error {
	if pool == nil || request.ExpectedRevision < 0 || request.Plan.Version != campaignplan.PlanVersionV1 || request.Plan.Hash.IsZero() || len(request.Members) == 0 || len(request.Members) != len(request.Plan.Assignments) {
		return fmt.Errorf("invalid campaign plan persistence request")
	}
	if _, err := uuidParameter(request.CampaignID); err != nil {
		return fmt.Errorf("campaign plan campaign id: %w", err)
	}
	if _, err := uuidParameter(request.CandidateID); err != nil {
		return fmt.Errorf("campaign plan candidate id: %w", err)
	}
	for _, raw := range []string{request.PolicyVersionID, request.CanaryWaveID, request.LaterWaveID} {
		if _, err := uuidParameter(raw); err != nil {
			return fmt.Errorf("campaign plan identity: %w", err)
		}
	}
	assignments := make(map[string]campaignplan.Assignment, len(request.Plan.Assignments))
	for _, assignment := range request.Plan.Assignments {
		if _, exists := assignments[assignment.TargetID]; exists || assignment.TargetID == "" {
			return fmt.Errorf("invalid campaign plan assignment")
		}
		assignments[assignment.TargetID] = assignment
	}
	members := append([]CampaignPlanMember(nil), request.Members...)
	sort.Slice(members, func(i, j int) bool { return members[i].TargetID < members[j].TargetID })
	for index, member := range members {
		if _, err := uuidParameter(member.TargetID); err != nil {
			return fmt.Errorf("campaign plan target id: %w", err)
		}
		if _, err := uuidParameter(member.CampaignTargetID); err != nil {
			return fmt.Errorf("campaign plan membership id: %w", err)
		}
		if _, err := uuidParameter(member.TargetUpdateID); err != nil {
			return fmt.Errorf("campaign plan update id: %w", err)
		}
		if index > 0 && members[index-1].TargetID == member.TargetID {
			return fmt.Errorf("duplicate campaign plan member")
		}
		if _, exists := assignments[member.TargetID]; !exists {
			return fmt.Errorf("member absent from plan")
		}
	}
	policyHash := sha256.Sum256(request.Plan.PolicyJSON)
	policyDigest, err := value.ParseSHA256Digest("sha256:" + hex.EncodeToString(policyHash[:]))
	if err != nil {
		return fmt.Errorf("hash canonical policy: %w", err)
	}
	effective, err := json.Marshal(struct {
		Canary      uint64                    `json:"canary"`
		LaterWave   uint64                    `json:"later_wave"`
		Assignments []campaignplan.Assignment `json:"assignments"`
	}{request.Plan.EffectiveCanary, request.Plan.EffectiveLater, request.Plan.Assignments})
	if err != nil || len(effective) > 32*1024 {
		return fmt.Errorf("encode effective campaign plan")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin campaign plan persistence: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state, artifactID string
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT state,revision,artifact_id::text FROM campaigns WHERE id=$1::uuid FOR UPDATE`, request.CampaignID).Scan(&state, &revision, &artifactID); err != nil {
		return fmt.Errorf("lock campaign plan aggregate: %w", err)
	}
	if state != "DRAFT" || revision != request.ExpectedRevision {
		return ErrCampaignPlanConflict
	}
	var snapshotHash, artifactVersionID, artifactDigest, verificationPolicy string
	if err := tx.QueryRow(ctx, `SELECT c.snapshot_hash,c.artifact_version_id,o.sha256_digest,o.verification_policy_version FROM preflight_candidates c JOIN artifact_object_versions o ON o.artifact_id=c.artifact_id WHERE c.id=$1::uuid AND c.artifact_id=$2::uuid AND c.state='PASSED' FOR SHARE OF c,o`, request.CandidateID, artifactID).Scan(&snapshotHash, &artifactVersionID, &artifactDigest, &verificationPolicy); err != nil {
		return fmt.Errorf("load passed candidate for campaign plan: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET state='VALIDATING',revision=revision+1,updated_at=now() WHERE id=$1::uuid AND revision=$2 AND state='DRAFT'`, request.CampaignID, request.ExpectedRevision); err != nil {
		return fmt.Errorf("begin campaign validation: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO campaign_plan_snapshots (campaign_id,candidate_id,draft_revision,policy_json,effective_plan_json,seed,policy_version,algorithm_version,plan_version,artifact_version_id,artifact_digest,verification_policy_version,target_snapshot_hash,plan_hash) VALUES ($1::uuid,$2::uuid,$3,$4::jsonb,$5::jsonb,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, request.CampaignID, request.CandidateID, request.ExpectedRevision, string(request.Plan.PolicyJSON), string(effective), request.Plan.Seed, campaignplan.PolicyVersionV1, request.Plan.Algorithm, request.Plan.Version, artifactVersionID, artifactDigest, verificationPolicy, snapshotHash, request.Plan.Hash.String()); err != nil {
		return fmt.Errorf("insert immutable campaign plan snapshot: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO campaign_waves (id,campaign_id,ordinal,cohort) VALUES ($1::uuid,$2::uuid,1,'CANARY'),($3::uuid,$2::uuid,2,'LATER_WAVE')`, request.CanaryWaveID, request.CampaignID, request.LaterWaveID); err != nil {
		return fmt.Errorf("insert campaign waves: %w", err)
	}
	for _, member := range members {
		assignment := assignments[member.TargetID]
		waveID := request.CanaryWaveID
		if assignment.Cohort == campaignplan.CohortLaterWave {
			waveID = request.LaterWaveID
		}
		var frozenRevision int64
		if err := tx.QueryRow(ctx, `SELECT frozen_target_revision FROM preflight_candidate_targets WHERE candidate_id=$1::uuid AND target_id=$2::uuid FOR SHARE`, request.CandidateID, member.TargetID).Scan(&frozenRevision); err != nil {
			return fmt.Errorf("validate candidate membership %s: %w", member.TargetID, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO campaign_targets (id,campaign_id,target_id,artifact_id,frozen_target_revision,frozen_policy_hash,wave_id,cohort,cohort_ordinal,selection_reason) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7::uuid,$8,$9,$10)`, member.CampaignTargetID, request.CampaignID, member.TargetID, artifactID, frozenRevision, policyDigest.String(), waveID, assignment.Cohort, int64(assignment.Ordinal), assignment.Reason.String()); err != nil {
			return fmt.Errorf("insert campaign plan member %s: %w", member.TargetID, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO target_updates (id,campaign_target_id,state,expected_firmware_version) VALUES ($1::uuid,$2::uuid,'PENDING',$3)`, member.TargetUpdateID, member.CampaignTargetID, request.Plan.ExpectedFirmwareVersion); err != nil {
			return fmt.Errorf("insert target update %s: %w", member.TargetID, err)
		}
	}
	var candidateCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM preflight_candidate_targets WHERE candidate_id=$1::uuid`, request.CandidateID).Scan(&candidateCount); err != nil {
		return fmt.Errorf("count candidate membership: %w", err)
	}
	if candidateCount != len(members) {
		return ErrCampaignPlanConflict
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: campaign plan serialization", ErrCampaignPlanConflict)
		}
		return fmt.Errorf("commit campaign plan persistence: %w", err)
	}
	return nil
}
