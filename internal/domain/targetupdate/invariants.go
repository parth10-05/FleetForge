package targetupdate

// ValidateAggregate is the reusable target-update structural invariant for
// reducers and later persistence/application boundaries. It does not infer
// evidence or authorization.
func ValidateAggregate(aggregate Aggregate) bool {
	if !knownState(aggregate.State) {
		return false
	}
	if aggregate.State == StateRetryWait {
		_, err := ParseResumeStep(aggregate.ResumeStep.String())
		return err == nil
	}
	return aggregate.ResumeStep == ""
}
