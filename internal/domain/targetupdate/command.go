package targetupdate

import "errors"

var ErrUnknownCommand = errors.New("unknown target update command")

type Command string

const (
	CommandStartPrecheck            Command = "START_PRECHECK"
	CommandPrecheckPassed           Command = "PRECHECK_PASSED"
	CommandPrecheckRetryable        Command = "PRECHECK_RETRYABLE"
	CommandPrecheckTerminal         Command = "PRECHECK_TERMINAL"
	CommandAlreadyCurrent           Command = "ALREADY_CURRENT"
	CommandAdmitSubmission          Command = "ADMIT_SUBMISSION"
	CommandRemoteAccepted           Command = "REMOTE_ACCEPTED"
	CommandPreSendAbsent            Command = "PRE_SEND_PROVEN_ABSENT"
	CommandPostSendAmbiguous        Command = "POST_SEND_AMBIGUOUS"
	CommandRebootObserved           Command = "REBOOT_OBSERVED"
	CommandNoRebootRequired         Command = "NO_REBOOT_REQUIRED"
	CommandVerificationSucceeded    Command = "VERIFICATION_SUCCEEDED"
	CommandVerificationRetryable    Command = "VERIFICATION_RETRYABLE"
	CommandVerificationContradicted Command = "VERIFICATION_CONTRADICTED"
	CommandRetryDue                 Command = "RETRY_DUE"
	CommandAdoptOperation           Command = "ADOPT_OPERATION"
	CommandAttentionRetry           Command = "ATTENTION_RETRY_PROVEN_ABSENT"
	CommandAttentionTerminal        Command = "ATTENTION_TERMINAL_FAILURE"
	CommandCancelNoSideEffect       Command = "CANCEL_NO_SIDE_EFFECT"
	CommandContinueObservation      Command = "CONTINUE_OBSERVATION"
)

var allCommands = []Command{CommandStartPrecheck, CommandPrecheckPassed, CommandPrecheckRetryable, CommandPrecheckTerminal, CommandAlreadyCurrent, CommandAdmitSubmission, CommandRemoteAccepted, CommandPreSendAbsent, CommandPostSendAmbiguous, CommandRebootObserved, CommandNoRebootRequired, CommandVerificationSucceeded, CommandVerificationRetryable, CommandVerificationContradicted, CommandRetryDue, CommandAdoptOperation, CommandAttentionRetry, CommandAttentionTerminal, CommandCancelNoSideEffect, CommandContinueObservation}

func ParseCommand(raw string) (Command, error) {
	for _, command := range allCommands {
		if raw == string(command) {
			return command, nil
		}
	}
	return "", ErrUnknownCommand
}
func (command Command) String() string { return string(command) }
func AllCommands() []Command           { return append([]Command(nil), allCommands...) }
