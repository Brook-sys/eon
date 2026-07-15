package spike

import (
	"context"
	"fmt"
	"os/exec"

	"motor-autonomo/internal/port"
)

type CrashCommand struct {
	Executable string
	Args       []string
	Env        []string
}

type StoreOpener func() (port.Store, func() error, error)

type CrashTrialResult struct {
	ExitError string       `json:"exit_error,omitempty"`
	Outcome   CrashOutcome `json:"outcome"`
}

// RunCrashTrial executes the mutating worker in a distinct process, then opens
// the durable backend again through a fresh adapter and classifies visibility.
func RunCrashTrial(ctx context.Context, command CrashCommand, open StoreOpener, intent CrashIntent) (CrashTrialResult, error) {
	if command.Executable == "" || open == nil {
		return CrashTrialResult{}, fmt.Errorf("crash command and store opener are required")
	}
	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Env = command.Env
	err := cmd.Run()
	result := CrashTrialResult{}
	if err != nil {
		result.ExitError = err.Error()
	}
	store, closeStore, openErr := open()
	if openErr != nil {
		return result, fmt.Errorf("reopen crash backend: %w", openErr)
	}
	if closeStore != nil {
		defer closeStore()
	}
	outcome, inspectErr := InspectCrashIntent(ctx, store, intent)
	if inspectErr != nil {
		return result, inspectErr
	}
	result.Outcome = outcome
	return result, nil
}
