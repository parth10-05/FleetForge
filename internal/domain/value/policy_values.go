package value

import "errors"

var (
	ErrInvalidRetryBudget      = errors.New("retry budget must be positive")
	ErrInvalidFailureBudget    = errors.New("invalid failure budget")
	ErrInvalidConcurrencyLimit = errors.New("concurrency limit must be positive")
)

// RetryBudget bounds the total attempts permitted for one target-update step.
type RetryBudget struct {
	maxAttempts uint64
}

func NewRetryBudget(raw int64) (RetryBudget, error) {
	if raw <= 0 {
		return RetryBudget{}, ErrInvalidRetryBudget
	}
	return RetryBudget{maxAttempts: uint64(raw)}, nil
}

func (budget RetryBudget) MaxAttempts() uint64 { return budget.maxAttempts }

// FailureBudget is an exact non-negative rational threshold. Policy evaluation
// and its inclusive boundary are implemented in the later policy slice.
type FailureBudget struct {
	numerator   uint64
	denominator uint64
}

func NewFailureBudget(numerator, denominator int64) (FailureBudget, error) {
	if numerator < 0 || denominator <= 0 || numerator > denominator {
		return FailureBudget{}, ErrInvalidFailureBudget
	}
	return FailureBudget{numerator: uint64(numerator), denominator: uint64(denominator)}, nil
}

func (budget FailureBudget) Numerator() uint64 { return budget.numerator }

func (budget FailureBudget) Denominator() uint64 { return budget.denominator }

// ConcurrencyLimit is a positive safety limit. Scope-specific use belongs to
// later planning and orchestration phases.
type ConcurrencyLimit struct {
	value uint64
}

func NewConcurrencyLimit(raw int64) (ConcurrencyLimit, error) {
	if raw <= 0 {
		return ConcurrencyLimit{}, ErrInvalidConcurrencyLimit
	}
	return ConcurrencyLimit{value: uint64(raw)}, nil
}

func (limit ConcurrencyLimit) Uint64() uint64 { return limit.value }
