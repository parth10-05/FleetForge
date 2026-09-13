// Package canary defines the deterministic selection contract for a frozen
// candidate set. Candidate discovery and policy-specific stratification belong
// to later phases.
package canary

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"unicode"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const AlgorithmSHA256V1 = "sha256-v1"

type Candidate struct {
	TargetID value.TargetID
	Domains  value.FailureDomainLabels
}

type SelectionInput struct {
	Algorithm  string
	Seed       string
	Count      uint64
	Candidates []Candidate
}

type Selection struct {
	Valid      bool
	Algorithm  string
	Provenance string
	Selected   []Candidate
}

// Select ranks a frozen, unique candidate set by SHA-256 over the explicit
// seed, target identity, and canonicalized failure-domain labels. The result
// is independent of caller order and contains no random or database behavior.
func Select(input SelectionInput) Selection {
	if input.Algorithm != AlgorithmSHA256V1 || !validSeed(input.Seed) || input.Count == 0 || input.Count > uint64(len(input.Candidates)) {
		return Selection{}
	}
	ranked := make([]rankedCandidate, 0, len(input.Candidates))
	seen := make(map[string]struct{}, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.TargetID.IsZero() {
			return Selection{}
		}
		id := candidate.TargetID.String()
		if _, duplicate := seen[id]; duplicate {
			return Selection{}
		}
		seen[id] = struct{}{}
		domain := canonicalDomain(candidate.Domains)
		if domain == "" {
			return Selection{}
		}
		digest := sha256.Sum256([]byte(input.Seed + "\x00" + id + "\x00" + domain))
		ranked = append(ranked, rankedCandidate{candidate: candidate, digest: hex.EncodeToString(digest[:])})
	}
	sort.Slice(ranked, func(left, right int) bool {
		if ranked[left].digest == ranked[right].digest {
			return ranked[left].candidate.TargetID.String() < ranked[right].candidate.TargetID.String()
		}
		return ranked[left].digest < ranked[right].digest
	})
	selected := make([]Candidate, input.Count)
	for index := range selected {
		selected[index] = ranked[index].candidate
	}
	return Selection{Valid: true, Algorithm: input.Algorithm, Provenance: provenance(input.Algorithm, input.Seed, ranked), Selected: selected}
}

type rankedCandidate struct {
	candidate Candidate
	digest    string
}

func canonicalDomain(labels value.FailureDomainLabels) string {
	raw := labels.AsMap()
	if len(raw) == 0 {
		return ""
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, strings.ToLower(key)+"="+strings.ToLower(raw[key]))
	}
	return strings.Join(parts, "\x00")
}

func provenance(algorithm, seed string, ranked []rankedCandidate) string {
	hash := sha256.New()
	hash.Write([]byte(algorithm))
	hash.Write([]byte("\x00" + seed))
	for _, item := range ranked {
		hash.Write([]byte("\x00" + item.candidate.TargetID.String() + "\x00" + item.digest))
	}
	return algorithm + ":" + hex.EncodeToString(hash.Sum(nil))
}

func validSeed(seed string) bool {
	if len(seed) == 0 || len(seed) > 128 {
		return false
	}
	for _, character := range seed {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
