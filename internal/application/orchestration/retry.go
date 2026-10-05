package orchestration

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// RetryAt deterministically derives bounded exponential backoff from permanent
// command identity, delivery attempt, and an explicit schedule version/salt.
// It deliberately has no process-global random dependency, so replay and
// bounded fairness tests reconstruct the same eligibility time.
func RetryAt(now time.Time, commandID string, attempt int64, base, maximum time.Duration, salt string) (time.Time, error) {
	if now.IsZero() || commandID == "" || attempt <= 0 || base <= 0 || maximum < base || salt == "" {
		return time.Time{}, fmt.Errorf("invalid retry schedule input")
	}
	delay := base
	for step := int64(1); step < attempt && delay < maximum; step++ {
		if delay > maximum/2 {
			delay = maximum
			break
		}
		delay *= 2
	}
	if delay > maximum {
		delay = maximum
	}
	// Jitter is 0..10% of the capped deterministic delay.
	digest := sha256.Sum256([]byte(salt + ":" + commandID + ":" + fmt.Sprint(attempt)))
	fraction := binary.BigEndian.Uint64(digest[:8]) % 1001
	jitter := time.Duration((int64(delay) * int64(fraction)) / 10_000)
	return now.UTC().Add(delay + jitter), nil
}
