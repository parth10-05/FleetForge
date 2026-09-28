package inventory

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strconv"
	"time"
)

var ErrInvalidRefreshSchedule = errors.New("invalid inventory refresh schedule")

// NextRefresh applies stable positive jitter of at most ten percent. The same
// target/revision/interval always yields the same time, avoiding coordinated
// polls without introducing a process-random scheduling decision.
func NextRefresh(targetKey string, revision int64, now time.Time, interval time.Duration) (time.Time, error) {
	if targetKey == "" || revision < 0 || now.IsZero() || interval <= 0 {
		return time.Time{}, ErrInvalidRefreshSchedule
	}
	sum := sha256.Sum256([]byte(targetKey + "\x00" + strconv.FormatInt(revision, 10)))
	fraction := binary.BigEndian.Uint64(sum[:8]) % 1001 // 0..1000 basis points
	jitter := time.Duration((int64(interval) * int64(fraction)) / 10_000)
	return now.UTC().Add(interval + jitter), nil
}
