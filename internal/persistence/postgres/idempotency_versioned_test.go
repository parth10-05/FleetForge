package postgres

import (
	"context"
	"strings"
	"testing"
)

func TestCompleteVersionedIdempotencyKeyRejectsMalformedReferenceBeforeDatabaseAccess(t *testing.T) {
	err := CompleteVersionedIdempotencyKey(context.Background(), nil, IdempotencyCompletion{
		ResponseReference: "not-a-versioned-reference",
	}, "owner-token")
	if err == nil || !strings.Contains(err.Error(), "validate versioned idempotency response reference") {
		t.Fatalf("CompleteVersionedIdempotencyKey() error = %v, want response-reference validation error", err)
	}
}

func TestApplyVersionedIdempotentAggregateMutationRejectsMalformedReferenceBeforeDatabaseAccess(t *testing.T) {
	err := ApplyVersionedIdempotentAggregateMutation(context.Background(), nil, IdempotentAggregateMutation{
		Completion: IdempotencyCompletion{ResponseReference: "not-a-versioned-reference"},
	})
	if err == nil || !strings.Contains(err.Error(), "validate versioned idempotency response reference") {
		t.Fatalf("ApplyVersionedIdempotentAggregateMutation() error = %v, want response-reference validation error", err)
	}
}
