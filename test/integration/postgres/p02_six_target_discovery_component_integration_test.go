//go:build postgresintegration

package postgres

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appartifacts "github.com/parth10-05/fleetforge/internal/application/artifacts"
	appinventory "github.com/parth10-05/fleetforge/internal/application/inventory"
	apppreflight "github.com/parth10-05/fleetforge/internal/application/preflight"
	"github.com/parth10-05/fleetforge/internal/delivery/inventoryadapter"
	"github.com/parth10-05/fleetforge/internal/delivery/redfishsim"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticdiscovery"
	"github.com/parth10-05/fleetforge/internal/redfish"
	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

// TestP02SixTargetDiscoveryComponentIsReadOnly composes the P02 registration
// and discovery adapters with P01's authenticated six-target simulator. It
// deliberately never reaches UpdateService: the retained independent journal
// must stay empty of firmware requests and accepted operations.
func TestP02SixTargetDiscoveryComponentIsReadOnly(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "simulator", "v1", "healthy.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := scenario.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := core.Start(s, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	journalPath := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := journal.OpenWriter(journalPath, "p02-six-target-discovery")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	server := httptest.NewServer(redfishsim.NewWithJournal(engine, []byte("user"), []byte("pass"), time.Second, 1024, writer).Routes())
	defer server.Close()

	credential, err := syntheticcredentials.NewCredential("user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	credentials := make(map[string]syntheticcredentials.Credential, len(s.Targets))
	keys := make([]string, 0, len(s.Targets))
	for _, target := range s.Targets {
		key, idErr := scenario.ID(s, "target", target.Index)
		if idErr != nil {
			t.Fatal(idErr)
		}
		keys = append(keys, key)
		credentials[key] = credential
	}
	resolver, err := syntheticcredentials.New(credentials)
	if err != nil {
		t.Fatal(err)
	}
	registration := appinventory.New(inventoryadapter.PostgresRegistrationWriter{Pool: pool})
	now := time.Now().UTC()
	for index, target := range s.Targets {
		targetID := fmt.Sprintf("00000000-0000-0000-3001-%012d", index+1)
		if _, err := registration.RegisterTarget(ctx, appinventory.RegisterTargetRequest{TargetID: targetID, ExternalID: "component-" + keys[index], Origin: server.URL, TargetKey: keys[index], CredentialReference: "synthetic-test:" + keys[index], TransportPolicy: "synthetic-allowlisted-v1", Labels: map[string]string{"site": target.Site, "rack": target.Rack, "family": "synthetic", "group": "component-six"}}); err != nil {
			t.Fatalf("register target %d: %v", index+1, err)
		}
		workID := fmt.Sprintf("00000000-0000-0000-3002-%012d", index+1)
		commandID := fmt.Sprintf("00000000-0000-0000-3003-%012d", index+1)
		if ok, err := postgres.ScheduleDiscovery(ctx, pool, postgres.DiscoveryScheduleRequest{WorkID: workID, CommandID: commandID, TargetID: targetID, TargetRevision: 0, NotBefore: now}); err != nil || !ok {
			t.Fatalf("schedule target %d: ok=%t err=%v", index+1, ok, err)
		}
	}

	adapter := postgres.DiscoveryWorkAdapter{Pool: pool}
	observation := 0
	executor := syntheticdiscovery.Executor{Loader: adapter, Client: redfish.NewClient(redfish.ClientConfig{HTTPClient: server.Client(), AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}), Resolver: resolver, Now: func() time.Time { return now }, NewObservationID: func() (string, error) {
		observation++
		return fmt.Sprintf("00000000-0000-0000-3004-%012d", observation), nil
	}}
	for i := 0; i < len(s.Targets); i++ {
		processed, err := appinventory.RunDiscoveryOnce(ctx, adapter, executor, adapter, appinventory.DiscoveryClaimRequest{Owner: "component-worker", LeaseToken: fmt.Sprintf("00000000-0000-0000-3005-%012d", i+1), Now: now, LeaseExpiresAt: now.Add(time.Minute)}, retryOneSecond)
		if err != nil || !processed {
			t.Fatalf("discover target %d: processed=%t err=%v", i+1, processed, err)
		}
	}
	var heads, completed int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM target_discovery_heads").Scan(&heads); err != nil || heads != 6 {
		t.Fatalf("discovery heads=%d err=%v", heads, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM due_work WHERE work_type='DISCOVER' AND delivery_state='COMPLETED'").Scan(&completed); err != nil || completed != 6 {
		t.Fatalf("completed discovery work=%d err=%v", completed, err)
	}

	// The artifact path uses the actual temporary object store, immutable
	// binding transaction, verification claim, and streaming worker. No target
	// operation is admitted or submitted by this P02-only component test.
	const artifactID = "00000000-0000-0000-3006-000000000001"
	digest, err := value.ParseSHA256Digest("sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	if err != nil {
		t.Fatal(err)
	}
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/component.bin", "component-version-1", 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	store, err := localartifactstore.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(object, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ($1,'v1',$2,$3,'UPLOADING')", artifactID, digest.String(), object.Key()); err != nil {
		t.Fatal(err)
	}
	if err := postgres.BindUploadedArtifactObject(ctx, pool, postgres.ArtifactBindingRequest{ArtifactID: artifactID, Object: object}); err != nil {
		t.Fatal(err)
	}
	if ok, err := postgres.StartAndScheduleArtifactVerification(ctx, pool, postgres.ArtifactVerificationStartRequest{ArtifactID: artifactID, WorkID: "00000000-0000-0000-3006-000000000002", CommandID: "00000000-0000-0000-3006-000000000003", NotBefore: now}); err != nil || !ok {
		t.Fatalf("schedule verification: ok=%t err=%v", ok, err)
	}
	verification := postgres.ArtifactVerificationAdapter{Pool: pool}
	verificationService := appartifacts.NewWithCompletion(nil, nil, store, nil, func() time.Time { return now }).WithVerification(verification, verification)
	verificationWork := postgres.ArtifactVerificationWorkAdapter{Pool: pool}
	worker := appartifacts.NewWorker(verificationService, verificationWork, verificationWork, func() time.Time { return now })
	if err := worker.RunOnce(ctx, appartifacts.VerificationWorkClaimRequest{Owner: "component-verifier", LeaseToken: "00000000-0000-0000-3006-000000000004", Now: now, LeaseExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var artifactState string
	if err := pool.QueryRow(ctx, "SELECT state FROM firmware_artifacts WHERE id=$1::uuid", artifactID).Scan(&artifactState); err != nil || artifactState != "VERIFIED" {
		t.Fatalf("artifact state=%q err=%v", artifactState, err)
	}

	// The real preflight service freezes all six discovered targets against the
	// exact verified artifact object. A later live-family change must block a
	// fresh candidate and be reported as drift without mutating this snapshot.
	parsedArtifact, err := value.ParseArtifactID(artifactID)
	if err != nil {
		t.Fatal(err)
	}
	preflightAdapter := postgres.PreflightAdapter{Pool: pool}
	preflightService := apppreflight.New(preflightAdapter, preflightAdapter, preflightAdapter)
	selector := []byte(`{"version":"fleetforge.selector/v1","all":[{"key":"group","value":"component-six"}]}`)
	result, err := preflightService.Evaluate(ctx, apppreflight.Request{CandidateID: "00000000-0000-0000-3007-000000000001", ArtifactID: parsedArtifact, Selector: selector, Maximum: 6})
	if err != nil || len(result.Snapshot.Targets()) != 6 {
		t.Fatalf("six-target preflight: targets=%d err=%v", len(result.Snapshot.Targets()), err)
	}
	if _, err := pool.Exec(ctx, "UPDATE target_labels SET value='unsupported' WHERE target_id=$1::uuid AND key='family'", "00000000-0000-0000-3001-000000000001"); err != nil {
		t.Fatal(err)
	}
	if _, err := preflightService.Evaluate(ctx, apppreflight.Request{CandidateID: "00000000-0000-0000-3007-000000000002", ArtifactID: parsedArtifact, Selector: selector, Maximum: 6}); !errors.Is(err, apppreflight.ErrBlocked) {
		t.Fatalf("incompatible live family error=%v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE targets SET revision=revision+1 WHERE id=$1::uuid", "00000000-0000-0000-3001-000000000001"); err != nil {
		t.Fatal(err)
	}
	drift, err := postgres.ReportPreflightCandidateDrift(ctx, pool, "00000000-0000-0000-3007-000000000001")
	if err != nil || len(drift) != 1 || drift[0].TargetID != "00000000-0000-0000-3001-000000000001" || len(drift[0].Reasons) != 2 || drift[0].Reasons[0] != "revision_changed" || drift[0].Reasons[1] != "family_changed" {
		t.Fatalf("frozen-candidate drift=%+v err=%v", drift, err)
	}

	// Cleanup receives only a durable grant and resolves its exact object tuple
	// from the local store after the expiration decision commits. It removes one
	// abandoned upload while retaining the verified object frozen by preflight.
	const abandonedID = "00000000-0000-0000-3008-000000000001"
	const abandonedGrantID = "component-abandoned-grant"
	cleanupSecret := []byte("01234567890123456789012345678901")
	abandonedClaims := localartifactstore.UploadGrantClaims{Version: "fleetforge.upload-grant/v1", GrantID: abandonedGrantID, ArtifactID: abandonedID, Method: "PUT", Provider: artifact.ObjectProviderLocalV1, Container: "artifacts", Key: "artifacts/v1/abandoned.bin", VersionID: "component-abandoned-version-1", Size: 3, Digest: digest.String(), MediaType: artifact.SyntheticFirmwareMediaType, ExpiresAt: now.Add(time.Minute)}
	abandonedGrant, err := localartifactstore.IssueSignedGrant(cleanupSecret, abandonedClaims)
	if err != nil {
		t.Fatal(err)
	}
	abandonedObject, err := store.PutSignedGrant(cleanupSecret, abandonedGrant, now, strings.NewReader("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,upload_grant_id,state,created_at) VALUES ($1,'v1',$2,'pending',$3,'UPLOADING',$4)", abandonedID, digest.String(), abandonedGrantID, now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	expired, err := postgres.ExpireAbandonedArtifactUploads(ctx, pool, now.Add(-24*time.Hour), 1)
	if err != nil || len(expired) != 1 || expired[0].ArtifactID != abandonedID || expired[0].GrantID != abandonedGrantID {
		t.Fatalf("expire abandoned upload=%+v err=%v", expired, err)
	}
	cleanupAdapter := postgres.ArtifactCleanupIntentAdapter{Pool: pool}
	processed, err := appartifacts.RunCleanupOnce(ctx, cleanupAdapter, cleanupAdapter, store, store, "component-cleanup", "00000000-0000-0000-3008-000000000002", now, now.Add(time.Minute))
	if err != nil || !processed {
		t.Fatalf("cleanup abandoned upload: processed=%t err=%v", processed, err)
	}
	if _, err := store.Open(abandonedObject); err == nil {
		t.Fatal("cleanup retained abandoned exact object")
	}
	verifiedReader, err := store.Open(object)
	if err != nil {
		t.Fatalf("cleanup removed verified object: %v", err)
	}
	if err := verifiedReader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.Load(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Kind == journal.RequestReceived || entry.Kind == journal.UpdateAccepted {
			t.Fatalf("P02 read-only component wrote firmware journal entry: %#v", entry)
		}
	}
}
