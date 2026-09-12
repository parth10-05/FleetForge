package artifact

// IsTerminal reports whether an artifact can never become eligible again under
// this lifecycle. Creating and verifying a replacement is a separate artifact
// operation, not a transition out of a terminal state.
func (state State) IsTerminal() bool {
	return state == StateRejected || state == StateRetired || state == StateExpired
}

// CanTransition defines the in-memory artifact lifecycle only. It neither
// invokes object storage nor evaluates integrity evidence.
func CanTransition(from, to State) bool {
	switch from {
	case StateInitiated:
		return to == StateUploading
	case StateUploading:
		return to == StateUploaded
	case StateUploaded:
		return to == StateVerifying
	case StateVerifying:
		return to == StateVerified || to == StateRejected
	case StateVerified:
		return to == StateRetired || to == StateExpired
	default:
		return false
	}
}

// IsReadyForNewCampaign is intentionally narrower than lifecycle validity.
// Existing running-campaign evidence is handled in a later persistence phase.
func (state State) IsReadyForNewCampaign() bool { return state == StateVerified }
