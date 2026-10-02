// Package preflight owns canonical immutable target/artifact candidate facts.
package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const SnapshotVersionV1 = "v1-p02-snapshot-v1"

var ErrInvalidSnapshot = errors.New("invalid frozen preflight snapshot")

type Target struct {
	ID            value.TargetID
	Revision      value.Revision
	Labels        value.FailureDomainLabels
	Family        value.LabelValue
	Compatibility artifact.CompatibilityDecision
}

// Snapshot holds only explicit, canonicalized facts. It never re-evaluates
// live inventory and therefore remains stable after later inventory drift.
type Snapshot struct {
	selectorVersion string
	selector        []byte
	artifactID      value.ArtifactID
	object          artifact.ObjectIdentity
	targets         []Target
	hash            value.SHA256Digest
}

func NewSnapshot(selector inventory.Selector, artifactID value.ArtifactID, object artifact.ObjectIdentity, targets []Target) (Snapshot, error) {
	if artifactID.IsZero() || object.VersionID() == "" || len(targets) == 0 || len(targets) > 100 {
		return Snapshot{}, ErrInvalidSnapshot
	}
	canonicalTargets := append([]Target(nil), targets...)
	sort.Slice(canonicalTargets, func(i, j int) bool { return canonicalTargets[i].ID.String() < canonicalTargets[j].ID.String() })
	for index, target := range canonicalTargets {
		if target.ID.IsZero() || target.Family.IsZero() || !target.Compatibility.Compatible() || target.Compatibility.PolicyVersion() == "" {
			return Snapshot{}, ErrInvalidSnapshot
		}
		if index > 0 && canonicalTargets[index-1].ID.String() == target.ID.String() {
			return Snapshot{}, ErrInvalidSnapshot
		}
	}
	canonical, err := marshalCanonical(selector, artifactID, object, canonicalTargets)
	if err != nil {
		return Snapshot{}, ErrInvalidSnapshot
	}
	digest := sha256.Sum256(canonical)
	hash, err := value.ParseSHA256Digest("sha256:" + hex.EncodeToString(digest[:]))
	if err != nil {
		return Snapshot{}, ErrInvalidSnapshot
	}
	return Snapshot{selectorVersion: inventory.SelectorGrammarV1, selector: selector.CanonicalJSON(), artifactID: artifactID, object: object, targets: canonicalTargets, hash: hash}, nil
}

func (snapshot Snapshot) Hash() value.SHA256Digest                { return snapshot.hash }
func (snapshot Snapshot) SelectorVersion() string                 { return snapshot.selectorVersion }
func (snapshot Snapshot) Selector() []byte                        { return append([]byte(nil), snapshot.selector...) }
func (snapshot Snapshot) ArtifactID() value.ArtifactID            { return snapshot.artifactID }
func (snapshot Snapshot) ObjectIdentity() artifact.ObjectIdentity { return snapshot.object }
func (snapshot Snapshot) Targets() []Target                       { return append([]Target(nil), snapshot.targets...) }

func marshalCanonical(selector inventory.Selector, artifactID value.ArtifactID, object artifact.ObjectIdentity, targets []Target) ([]byte, error) {
	type canonicalTarget struct {
		ID       string `json:"id"`
		Revision uint64 `json:"revision"`
		Site     string `json:"site"`
		Rack     string `json:"rack"`
		Family   string `json:"family"`
		Policy   string `json:"compatibility_policy"`
	}
	type canonicalObject struct {
		Provider  string `json:"provider"`
		Container string `json:"container"`
		Key       string `json:"key"`
		Version   string `json:"version"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	}
	result := struct {
		Version    string            `json:"version"`
		Selector   json.RawMessage   `json:"selector"`
		ArtifactID string            `json:"artifact_id"`
		Object     canonicalObject   `json:"object"`
		Targets    []canonicalTarget `json:"targets"`
	}{
		Version: SnapshotVersionV1, Selector: selector.CanonicalJSON(), ArtifactID: artifactID.String(),
		Object: canonicalObject{Provider: object.Provider(), Container: object.Container(), Key: object.Key(), Version: object.VersionID(), Size: object.Size(), Digest: object.Digest().String()},
	}
	for _, target := range targets {
		result.Targets = append(result.Targets, canonicalTarget{ID: target.ID.String(), Revision: target.Revision.Uint64(), Site: target.Labels.Site().String(), Rack: target.Labels.Rack().String(), Family: target.Family.String(), Policy: target.Compatibility.PolicyVersion()})
	}
	return json.Marshal(result)
}
