package targetupdate

import "github.com/parth10-05/fleetforge/internal/domain/reason"

type TransitionClass string

const (
	TransitionAllowed  TransitionClass = "ALLOWED"
	TransitionRejected TransitionClass = "REJECTED"
	TransitionNoOp     TransitionClass = "NO_OP"
)

type AttentionExit string

const (
	AttentionExitNone       AttentionExit = "NONE"
	AttentionExitAdopt      AttentionExit = "ADOPT_OPERATION"
	AttentionExitTerminal   AttentionExit = "TERMINAL_FAILURE"
	AttentionExitProofRetry AttentionExit = "PROOF_BACKED_RETRY"
)

type Transition struct {
	From         State
	Command      Command
	Class        TransitionClass
	Result       State
	Resume       ResumeStep
	Attention    AttentionExit
	Reason       reason.Code
	SubmitIntent bool
}
