package campaignplan

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"sort"

	"github.com/parth10-05/fleetforge/internal/domain/reason"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var ErrInvalidPlanInput = errors.New("invalid campaign plan input")

type Candidate struct {
	TargetID value.TargetID
	Revision value.Revision
	Site     string
	Rack     string
	Family   string
}
type Input struct {
	Policy     Policy
	Seed       string
	Algorithm  string
	Candidates []Candidate
}
type Cohort string

const (
	CohortCanary    Cohort = "CANARY"
	CohortLaterWave Cohort = "LATER_WAVE"
)

type Assignment struct {
	TargetID string      `json:"target_id"`
	Cohort   Cohort      `json:"cohort"`
	Ordinal  uint64      `json:"ordinal"`
	Reason   reason.Code `json:"reason"`
	Site     string      `json:"site"`
	Rack     string      `json:"rack"`
}
type Plan struct {
	Version                 string
	Algorithm               string
	Seed                    string
	ExpectedFirmwareVersion string
	EffectiveCanary         uint64
	EffectiveLater          uint64
	Assignments             []Assignment
	PolicyJSON              []byte
	Hash                    value.SHA256Digest
}

func Build(input Input) (Plan, error) {
	if input.Algorithm != SelectionVersionV1 || !validSeed(input.Seed) || len(input.Candidates) == 0 || len(input.Candidates) > maxCandidates {
		return Plan{}, ErrInvalidPlanInput
	}
	if err := input.Policy.Validate(uint64(len(input.Candidates))); err != nil {
		return Plan{}, ErrInvalidPlanInput
	}
	policyJSON, err := input.Policy.CanonicalJSON()
	if err != nil {
		return Plan{}, ErrInvalidPlanInput
	}
	ranked, err := rank(input.Algorithm, input.Seed, input.Candidates)
	if err != nil {
		return Plan{}, ErrInvalidPlanInput
	}
	canarySize, _ := input.Policy.EffectiveCanary(uint64(len(ranked)))
	selected, selectedRacks := make(map[string]struct{}, canarySize), make(map[string]struct{}, canarySize)
	assignments := make([]Assignment, 0, len(ranked))
	for _, candidate := range ranked {
		if uint64(len(selected)) == canarySize {
			break
		}
		rackKey := candidate.Site + "\x00" + candidate.Rack
		if _, exists := selectedRacks[rackKey]; !exists {
			selected[candidate.TargetID.String()] = struct{}{}
			selectedRacks[rackKey] = struct{}{}
			assignments = append(assignments, Assignment{TargetID: candidate.TargetID.String(), Cohort: CohortCanary, Ordinal: uint64(len(assignments)) + 1, Reason: reason.CodeCampaignPlanCanaryDiversity, Site: candidate.Site, Rack: candidate.Rack})
		}
	}
	for _, candidate := range ranked {
		if uint64(len(selected)) == canarySize {
			break
		}
		if _, exists := selected[candidate.TargetID.String()]; !exists {
			selected[candidate.TargetID.String()] = struct{}{}
			assignments = append(assignments, Assignment{TargetID: candidate.TargetID.String(), Cohort: CohortCanary, Ordinal: uint64(len(assignments)) + 1, Reason: reason.CodeCampaignPlanCanaryRankFill, Site: candidate.Site, Rack: candidate.Rack})
		}
	}
	laterOrdinal := uint64(0)
	for _, candidate := range ranked {
		if _, exists := selected[candidate.TargetID.String()]; exists {
			continue
		}
		laterOrdinal++
		assignments = append(assignments, Assignment{TargetID: candidate.TargetID.String(), Cohort: CohortLaterWave, Ordinal: laterOrdinal, Reason: reason.CodeCampaignPlanLaterWave, Site: candidate.Site, Rack: candidate.Rack})
	}
	if len(assignments) != len(ranked) {
		return Plan{}, ErrInvalidPlanInput
	}
	canonical, err := canonicalPlan(input.Algorithm, input.Seed, policyJSON, assignments)
	if err != nil {
		return Plan{}, ErrInvalidPlanInput
	}
	sum := sha256.Sum256(canonical)
	digest, err := value.ParseSHA256Digest("sha256:" + hex.EncodeToString(sum[:]))
	if err != nil {
		return Plan{}, ErrInvalidPlanInput
	}
	return Plan{Version: PlanVersionV1, Algorithm: input.Algorithm, Seed: input.Seed, ExpectedFirmwareVersion: input.Policy.ExpectedFirmwareVersion, EffectiveCanary: canarySize, EffectiveLater: uint64(len(ranked)) - canarySize, Assignments: assignments, PolicyJSON: policyJSON, Hash: digest}, nil
}

type rankedCandidate struct {
	Candidate
	rank [32]byte
}

func rank(algorithm, seed string, candidates []Candidate) ([]rankedCandidate, error) {
	ranked := make([]rankedCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		id := candidate.TargetID.String()
		if candidate.TargetID.IsZero() || candidate.Site == "" || candidate.Rack == "" || candidate.Family == "" {
			return nil, ErrInvalidPlanInput
		}
		if _, exists := seen[id]; exists {
			return nil, ErrInvalidPlanInput
		}
		seen[id] = struct{}{}
		hasher := sha256.New()
		for _, part := range []string{algorithm, seed, id} {
			writePart(hasher, part)
		}
		var digest [32]byte
		copy(digest[:], hasher.Sum(nil))
		ranked = append(ranked, rankedCandidate{Candidate: candidate, rank: digest})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if comparison := bytes.Compare(ranked[i].rank[:], ranked[j].rank[:]); comparison != 0 {
			return comparison < 0
		}
		return ranked[i].TargetID.String() < ranked[j].TargetID.String()
	})
	return ranked, nil
}
func canonicalPlan(algorithm, seed string, policy json.RawMessage, assignments []Assignment) ([]byte, error) {
	return json.Marshal(struct {
		Version     string          `json:"version"`
		Algorithm   string          `json:"algorithm"`
		Seed        string          `json:"seed"`
		Policy      json.RawMessage `json:"policy"`
		Assignments []Assignment    `json:"assignments"`
	}{Version: PlanVersionV1, Algorithm: algorithm, Seed: seed, Policy: policy, Assignments: assignments})
}
func writePart(hasher hash.Hash, raw string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
	_, _ = hasher.Write(length[:])
	_, _ = hasher.Write([]byte(raw))
}
func validSeed(seed string) bool {
	if seed == "" || len(seed) > maxSeedBytes {
		return false
	}
	for _, rune := range seed {
		if rune < 0x20 || rune == 0x7f {
			return false
		}
	}
	return true
}
