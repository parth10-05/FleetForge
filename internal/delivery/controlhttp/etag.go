package controlhttp

import (
	"fmt"
	"strconv"
	"strings"
)

func revisionETag(revision int64) (string, error) {
	if revision < 1 {
		return "", fmt.Errorf("invalid revision")
	}
	return fmt.Sprintf("\"rev-%d\"", revision), nil
}

func parseRevisionETag(value string) (int64, error) {
	if !strings.HasPrefix(value, "\"rev-") || !strings.HasSuffix(value, "\"") {
		return 0, fmt.Errorf("invalid revision ETag")
	}
	revision, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(value, "\"rev-"), "\""), 10, 64)
	if err != nil || revision < 1 || value != fmt.Sprintf("\"rev-%d\"", revision) {
		return 0, fmt.Errorf("invalid revision ETag")
	}
	return revision, nil
}
