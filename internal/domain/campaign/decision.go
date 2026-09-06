package campaign

type TransitionClass string

const (
	TransitionAllowed  TransitionClass = "ALLOWED"
	TransitionRejected TransitionClass = "REJECTED"
	TransitionNoOp     TransitionClass = "NO_OP"
)

func (class TransitionClass) String() string { return string(class) }

type StateResolution string

const (
	StateResolutionFixed            StateResolution = "FIXED"
	StateResolutionRestoreSuspended StateResolution = "RESTORE_SUSPENDED_PHASE"
)

type RevisionEffect string

const (
	RevisionIncrement RevisionEffect = "INCREMENT"
	RevisionUnchanged RevisionEffect = "UNCHANGED"
)

type EpochEffect string

const (
	EpochIncrement EpochEffect = "INCREMENT"
	EpochUnchanged EpochEffect = "UNCHANGED"
)

type SuspendedPhaseEffect string

const (
	SuspendedPhaseNone    SuspendedPhaseEffect = "NONE"
	SuspendedPhaseCapture SuspendedPhaseEffect = "CAPTURE_SOURCE_STATE"
	SuspendedPhaseRestore SuspendedPhaseEffect = "RESTORE"
)
