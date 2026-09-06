package value

import (
	"errors"
	"testing"
)

func TestRetryBudgetRequiresPositiveAttemptLimit(t *testing.T) {
	budget, err := NewRetryBudget(3)
	if err != nil || budget.MaxAttempts() != 3 {
		t.Fatalf("NewRetryBudget() = %d, %v", budget.MaxAttempts(), err)
	}
	for _, raw := range []int64{0, -1} {
		_, err := NewRetryBudget(raw)
		if !errors.Is(err, ErrInvalidRetryBudget) {
			t.Fatalf("NewRetryBudget(%d) error = %v, want ErrInvalidRetryBudget", raw, err)
		}
	}
}

func TestFailureBudgetRetainsExactRationalTerms(t *testing.T) {
	budget, err := NewFailureBudget(1, 4)
	if err != nil || budget.Numerator() != 1 || budget.Denominator() != 4 {
		t.Fatalf("NewFailureBudget() = %d/%d, %v", budget.Numerator(), budget.Denominator(), err)
	}
	zeroFailureBudget, err := NewFailureBudget(0, 1)
	if err != nil || zeroFailureBudget.Numerator() != 0 {
		t.Fatalf("zero numerator budget = %d/%d, %v", zeroFailureBudget.Numerator(), zeroFailureBudget.Denominator(), err)
	}
	for _, input := range [][2]int64{{-1, 1}, {1, 0}, {2, 1}} {
		_, err := NewFailureBudget(input[0], input[1])
		if !errors.Is(err, ErrInvalidFailureBudget) {
			t.Fatalf("NewFailureBudget(%d, %d) error = %v, want ErrInvalidFailureBudget", input[0], input[1], err)
		}
	}
}

func TestConcurrencyLimitRequiresPositiveValue(t *testing.T) {
	limit, err := NewConcurrencyLimit(4)
	if err != nil || limit.Uint64() != 4 {
		t.Fatalf("NewConcurrencyLimit() = %d, %v", limit.Uint64(), err)
	}
	for _, raw := range []int64{0, -1} {
		_, err := NewConcurrencyLimit(raw)
		if !errors.Is(err, ErrInvalidConcurrencyLimit) {
			t.Fatalf("NewConcurrencyLimit(%d) error = %v, want ErrInvalidConcurrencyLimit", raw, err)
		}
	}
}
