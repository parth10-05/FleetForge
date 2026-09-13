package artifact

import "errors"

var ErrUnknownState = errors.New("unknown artifact state")

// State is the verification and administrative lifecycle state of a firmware
// artifact. Object-storage behavior is intentionally outside this package.
type State string

const (
	StateInitiated State = "INITIATED"
	StateUploading State = "UPLOADING"
	StateUploaded  State = "UPLOADED"
	StateVerifying State = "VERIFYING"
	StateVerified  State = "VERIFIED"
	StateRejected  State = "REJECTED"
	StateRetired   State = "RETIRED"
	StateExpired   State = "EXPIRED"
)

var allStates = []State{
	StateInitiated,
	StateUploading,
	StateUploaded,
	StateVerifying,
	StateVerified,
	StateRejected,
	StateRetired,
	StateExpired,
}

func ParseState(raw string) (State, error) {
	state := State(raw)
	for _, candidate := range allStates {
		if state == candidate {
			return state, nil
		}
	}
	return "", ErrUnknownState
}

func (state State) String() string { return string(state) }

func AllStates() []State { return append([]State(nil), allStates...) }
