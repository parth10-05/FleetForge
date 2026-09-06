package campaign

import "errors"

var ErrUnknownCommand = errors.New("unknown campaign command")

type Command string

const (
	CommandBeginValidation  Command = "BEGIN_VALIDATION"
	CommandValidationPassed Command = "VALIDATION_PASSED"
	CommandValidationFailed Command = "VALIDATION_FAILED"
	CommandStartCanary      Command = "START_CANARY"
	CommandCanaryTerminal   Command = "CANARY_TERMINAL"
	CommandApproveCanary    Command = "APPROVE_CANARY"
	CommandReleaseNextWave  Command = "RELEASE_NEXT_WAVE"
	CommandComplete         Command = "COMPLETE"
	CommandPause            Command = "PAUSE"
	CommandResume           Command = "RESUME"
	CommandCancel           Command = "CANCEL"
	CommandAbort            Command = "ABORT"
	CommandAbortFailed      Command = "ABORT_FAILED"
	CommandAbortCancelled   Command = "ABORT_CANCELLED"
)

var allCommands = []Command{CommandBeginValidation, CommandValidationPassed, CommandValidationFailed, CommandStartCanary, CommandCanaryTerminal, CommandApproveCanary, CommandReleaseNextWave, CommandComplete, CommandPause, CommandResume, CommandCancel, CommandAbort, CommandAbortFailed, CommandAbortCancelled}

func ParseCommand(raw string) (Command, error) {
	command := Command(raw)
	for _, candidate := range allCommands {
		if command == candidate {
			return command, nil
		}
	}
	return "", ErrUnknownCommand
}

func (command Command) String() string { return string(command) }
func AllCommands() []Command           { return append([]Command(nil), allCommands...) }
