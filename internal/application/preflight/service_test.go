package preflight

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	domainpreflight "github.com/parth10-05/fleetforge/internal/domain/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestEvaluatePersistsCompatibleFrozenSnapshot(t *testing.T) {
	artifactID, target := testInputs(t, "synthetic")
	writer := &recordingWriter{}
	service := New(staticSelector{targets: []Target{target}}, staticArtifact{record: testArtifact(t, artifactID, "synthetic")}, writer)
	result, err := service.Evaluate(context.Background(), Request{CandidateID: "candidate-1", ArtifactID: artifactID, Maximum: 10, Selector: []byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]}`)})
	if err != nil || writer.id != "candidate-1" || writer.snapshot.Hash() != result.Snapshot.Hash() || string(result.Report) != `{"state":"passed","target_count":1}` {
		t.Fatalf("result=%+v writer=%+v err=%v", result, writer, err)
	}
}
func TestEvaluateBlocksIncompatibleTargetWithoutPersisting(t *testing.T) {
	artifactID, target := testInputs(t, "other")
	writer := &recordingWriter{}
	service := New(staticSelector{targets: []Target{target}}, staticArtifact{record: testArtifact(t, artifactID, "synthetic")}, writer)
	_, err := service.Evaluate(context.Background(), Request{CandidateID: "candidate-1", ArtifactID: artifactID, Maximum: 10, Selector: []byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]}`)})
	if !errors.Is(err, ErrBlocked) || writer.id != "" {
		t.Fatalf("err=%v writer=%+v", err, writer)
	}
}

type staticSelector struct{ targets []Target }

func (source staticSelector) Select(context.Context, inventory.Selector, int) ([]Target, error) {
	return source.targets, nil
}

type staticArtifact struct{ record Artifact }

func (source staticArtifact) Artifact(context.Context, value.ArtifactID) (Artifact, error) {
	return source.record, nil
}

type recordingWriter struct {
	id       string
	snapshot domainpreflight.Snapshot
	report   []byte
}

func (writer *recordingWriter) Persist(_ context.Context, id string, snapshot domainpreflight.Snapshot, report []byte) error {
	writer.id, writer.snapshot, writer.report = id, snapshot, report
	return nil
}
func testInputs(t *testing.T, family string) (value.ArtifactID, Target) {
	t.Helper()
	artifactID, _ := value.ParseArtifactID("11111111-1111-4111-8111-111111111111")
	targetID, _ := value.ParseTargetID("22222222-2222-4222-8222-222222222222")
	labels, err := value.NewFailureDomainLabels(map[string]string{"site": "west", "rack": "rack-a", "family": family})
	if err != nil {
		t.Fatal(err)
	}
	targetFamily, _ := value.ParseLabelValue(family)
	return artifactID, Target{ID: targetID, Revision: 1, Labels: labels, Family: targetFamily}
}
func testArtifact(t *testing.T, id value.ArtifactID, family string) Artifact {
	t.Helper()
	digest, _ := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	object, err := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "objects/firmware.bin", "version-1", 3, digest)
	if err != nil {
		t.Fatal(err)
	}
	parsedFamily, _ := value.ParseLabelValue(family)
	return Artifact{ID: id, State: artifact.StateVerified, Family: parsedFamily, Object: object}
}
