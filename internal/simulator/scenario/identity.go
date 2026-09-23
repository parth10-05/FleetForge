package scenario

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
)

// ID derives a complete SHA-256 identity, avoiding silent truncation.
func ID(s Scenario, kind string, ordinal int) (string, error) {
	if !validID(kind, 32) || ordinal < 0 {
		return "", invalid("identity input")
	}
	digest, err := Digest(s)
	if err != nil {
		return "", err
	}
	input := SchemaVersionV1 + "\x00" + digest + "\x00" + s.Seed + "\x00" + kind + "\x00" + strconv.Itoa(ordinal)
	sum := sha256.Sum256([]byte(input))
	return kind + "-" + hex.EncodeToString(sum[:]), nil
}

// ValidateIdentityUniqueness fails rather than shortening or silently
// extending IDs when a caller supplies an identity set with a collision.
func ValidateIdentityUniqueness(ids []string) error {
	seen := map[string]struct{}{}
	for _, id := range ids {
		if id == "" {
			return invalid("empty identity")
		}
		if _, found := seen[id]; found {
			return fmt.Errorf("%w: identity collision", ErrInvalidScenario)
		}
		seen[id] = struct{}{}
	}
	return nil
}
