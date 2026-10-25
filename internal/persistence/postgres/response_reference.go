package postgres

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// ResponseReference is the non-secret, bounded replay locator persisted with
// a completed idempotency claim. It contains no bearer token, raw key, body,
// upload URL, or credential material.
type ResponseReference struct {
	Operation, ResourceID, CommandID string
	Revision                         int64
}

func (reference ResponseReference) Encode() (string, error) {
	if !validReferenceToken(reference.Operation) || !validReferenceUUID(reference.ResourceID) || !validReferenceUUID(reference.CommandID) || reference.Revision < 0 {
		return "", fmt.Errorf("invalid idempotency response reference")
	}
	encoded := "v1ref:" + reference.Operation + ":" + reference.ResourceID + ":" + strconv.FormatInt(reference.Revision, 10) + ":" + reference.CommandID
	if len(encoded) > 512 {
		return "", fmt.Errorf("invalid idempotency response reference")
	}
	return encoded, nil
}

func DecodeResponseReference(value string) (ResponseReference, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 5 || parts[0] != "v1ref" {
		return ResponseReference{}, fmt.Errorf("invalid idempotency response reference")
	}
	revision, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || revision < 0 || parts[3] != strconv.FormatInt(revision, 10) || !validReferenceToken(parts[1]) || !validReferenceUUID(parts[2]) || !validReferenceUUID(parts[4]) {
		return ResponseReference{}, fmt.Errorf("invalid idempotency response reference")
	}
	return ResponseReference{Operation: parts[1], ResourceID: parts[2], Revision: revision, CommandID: parts[4]}, nil
}

func validReferenceToken(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func validReferenceUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}
