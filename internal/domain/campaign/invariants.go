package campaign

// ValidateAggregate is the reusable campaign structural invariant for reducers
// and later persistence/application boundaries. It performs no I/O.
func ValidateAggregate(aggregate Aggregate) bool {
	if !knownState(aggregate.State) {
		return false
	}
	if aggregate.State == StatePaused {
		return isSuspensible(aggregate.SuspendedPhase)
	}
	return aggregate.SuspendedPhase == ""
}
