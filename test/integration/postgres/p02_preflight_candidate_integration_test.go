//go:build postgresintegration

package postgres

import (
	"errors"
	"strings"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	"github.com/parth10-05/fleetforge/internal/domain/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestP02PreflightCandidatePersistsFrozenMembershipImmutably(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const artifactID = "00000000-0000-0000-2701-000000000001"
	const targetID = "00000000-0000-0000-2701-000000000002"
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ($1,'v1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','bound','VERIFIED')", artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ($1,'local-versioned-v1','artifacts','artifacts/v1/firmware.bin','version-1',3,'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','synthetic-sha256-v1','synthetic')", artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state,target_key,contract_version,credential_provider,credential_reference,transport_policy_version,registration_hash,trust_metadata_complete) VALUES ($1,'candidate-target','https://simulator.example','west','rack-a','ACTIVE','candidate-target','synthetic-redfish-v1','synthetic-test','synthetic-test:candidate-target','synthetic-transport-v1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',true)", targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO target_labels (target_id,key,value) VALUES ($1,'family','synthetic'),($1,'site','west'),($1,'rack','rack-a')", targetID); err != nil {
		t.Fatal(err)
	}
	const observationID = "00000000-0000-0000-2701-000000000005"
	if _, err := pool.Exec(ctx, "INSERT INTO device_observations (id,target_id,kind,observed_at,normalized_data) VALUES ($1,$2,'INVENTORY',now(),'{}')", observationID, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO target_discovery_heads (target_id,head_revision,configuration_revision,latest_attempt_id,last_good_id) VALUES ($1,1,0,$2,$2)", targetID, observationID); err != nil {
		t.Fatal(err)
	}
	parsedArtifact, err := value.ParseArtifactID(artifactID)
	if err != nil {
		t.Fatal(err)
	}
	adapter := persistence.PreflightAdapter{Pool: pool}
	service := app.New(adapter, adapter, adapter)
	result, err := service.Evaluate(ctx, app.Request{CandidateID: "00000000-0000-0000-2701-000000000009", ArtifactID: parsedArtifact, Selector: []byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]}`), Maximum: 10})
	if err != nil || len(result.Snapshot.Targets()) != 1 {
		t.Fatalf("application preflight result=%+v err=%v", result, err)
	}
	snapshot := mustCandidateSnapshot(t, artifactID, targetID, "rack-a")
	request := persistence.PreflightCandidateRequest{CandidateID: "00000000-0000-0000-2701-000000000003", Snapshot: snapshot, Report: []byte(`{"checks":{"state":"passed"}}`)}
	if err := persistence.PersistPreflightCandidate(ctx, pool, request); err != nil {
		t.Fatal(err)
	}
	if err := persistence.PersistPreflightCandidate(ctx, pool, request); err != nil {
		t.Fatalf("same snapshot replay: %v", err)
	}
	var hash string
	var count int
	if err := pool.QueryRow(ctx, "SELECT snapshot_hash FROM preflight_candidates WHERE id=$1", request.CandidateID).Scan(&hash); err != nil || hash != snapshot.Hash().String() {
		t.Fatalf("snapshot hash=%q err=%v", hash, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM preflight_candidate_targets WHERE candidate_id=$1", request.CandidateID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("frozen member count=%d err=%v", count, err)
	}
	conflicting := request
	conflicting.Snapshot = mustCandidateSnapshot(t, artifactID, targetID, "rack-b")
	if err := persistence.PersistPreflightCandidate(ctx, pool, conflicting); !errors.Is(err, persistence.ErrPreflightInputStale) {
		t.Fatalf("stale conflicting candidate error=%v", err)
	}
	if err := persistence.RecordDiscoveryFailure(ctx, pool, persistence.DiscoveryObservationRequest{ObservationID: "00000000-0000-0000-2701-000000000006", TargetID: targetID, ExpectedTargetRevision: 0, ObservedAt: time.Now().UTC(), NormalizedData: []byte(`{"error_kind":"timeout"}`)}); err != nil {
		t.Fatal(err)
	}
	failedEvidenceAttempt := request
	failedEvidenceAttempt.CandidateID = "00000000-0000-0000-2701-000000000004"
	if err := persistence.PersistPreflightCandidate(ctx, pool, failedEvidenceAttempt); !errors.Is(err, persistence.ErrPreflightInputStale) {
		t.Fatalf("latest discovery failure candidate error=%v", err)
	}
	if err := persistence.RecordDiscoverySuccess(ctx, pool, persistence.DiscoveryObservationRequest{ObservationID: "00000000-0000-0000-2701-000000000007", TargetID: targetID, ExpectedTargetRevision: 0, ObservedAt: time.Now().UTC(), NormalizedData: []byte(`{"reachable":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := persistence.RetireVerifiedArtifact(ctx, pool, artifactID); err != nil {
		t.Fatal(err)
	}
	retiredAttempt := request
	retiredAttempt.CandidateID = "00000000-0000-0000-2701-000000000008"
	if err := persistence.PersistPreflightCandidate(ctx, pool, retiredAttempt); !errors.Is(err, persistence.ErrPreflightInputStale) {
		t.Fatalf("retired artifact candidate error=%v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE targets SET revision=1 WHERE id=$1", targetID); err != nil {
		t.Fatal(err)
	}
	drift, err := persistence.ReportPreflightCandidateDrift(ctx, pool, request.CandidateID)
	if err != nil || len(drift) != 1 || drift[0].TargetID != targetID || len(drift[0].Reasons) != 1 || drift[0].Reasons[0] != "revision_changed" {
		t.Fatalf("drift=%+v err=%v", drift, err)
	}
	if err := pool.QueryRow(ctx, "SELECT frozen_target_revision FROM preflight_candidate_targets WHERE candidate_id=$1 AND target_id=$2", request.CandidateID, targetID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("frozen revision=%d err=%v", count, err)
	}
}

func mustCandidateSnapshot(t *testing.T, artifactID, targetID, rack string) preflight.Snapshot {
	t.Helper()
	selector, err := inventory.ParseSelector([]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/firmware.bin", "version-1", 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	parsedArtifact, err := value.ParseArtifactID(artifactID)
	if err != nil {
		t.Fatal(err)
	}
	parsedTarget, err := value.ParseTargetID(targetID)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := value.NewFailureDomainLabels(map[string]string{"site": "west", "rack": rack, "family": "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	family, err := value.ParseLabelValue("synthetic")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := artifact.EvaluateCompatibility(artifact.CompatibilityPolicyV1, artifact.StateVerified, family, family)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := preflight.NewSnapshot(selector, parsedArtifact, object, []preflight.Target{{ID: parsedTarget, Revision: 0, Labels: labels, Family: family, Compatibility: decision}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
