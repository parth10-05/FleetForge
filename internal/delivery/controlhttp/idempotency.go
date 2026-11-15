package controlhttp

import (
	"fmt"
	"strings"
)

const idempotencyHeader = "Idempotency-Key"

func idempotencyKey(value string) (string, error) {
	if len(value) < 16 || len(value) > 128 || strings.TrimSpace(value) != value {
		return "", fmt.Errorf("invalid idempotency key")
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("._-", character)) {
			return "", fmt.Errorf("invalid idempotency key")
		}
	}
	return value, nil
}
