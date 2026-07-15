package spike

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"motor-autonomo/internal/port"
)

type CrashCommand struct {
	Executable string
	Args       []string
	Env        []string
}

type StoreOpener func() (port.Store, func() error, error)

type VisibilityInspector func(context.Context, port.Store) (CrashOutcome, error)

type CrashTrialResult struct {
	ExitError     string       `json:"exit_error,omitempty"`
	WorkerCrashed bool         `json:"worker_crashed"`
	Outcome       CrashOutcome `json:"outcome"`
}

const MinCrashCampaignTrials = 30

type CrashTrialPlan func(index int) (CrashCommand, StoreOpener, CrashIntent, error)

type CrashOutcomeCounts struct {
	NotApplied     int `json:"not_applied"`
	Applied        int `json:"applied"`
	InvalidPartial int `json:"invalid_partial"`
}

type CrashCampaignResult struct {
	Trials []CrashTrialResult `json:"trials"`
	Counts CrashOutcomeCounts `json:"counts"`
	Passed bool               `json:"passed"`
}

// RunCrashTrial executes the mutating worker in a distinct process, then opens
// the durable backend again through a fresh adapter and classifies visibility.
func RunCrashTrial(ctx context.Context, command CrashCommand, open StoreOpener, intent CrashIntent) (CrashTrialResult, error) {
	return RunCrashTrialWithInspector(ctx, command, open, func(ctx context.Context, store port.Store) (CrashOutcome, error) {
		return InspectCrashIntent(ctx, store, intent)
	})
}

// RunCrashTrialWithInspector lets the harness classify a compound official
// mutation rather than reducing atomicity to one sentinel record.
func RunCrashTrialWithInspector(ctx context.Context, command CrashCommand, open StoreOpener, inspect VisibilityInspector) (CrashTrialResult, error) {
	if command.Executable == "" || open == nil || inspect == nil {
		return CrashTrialResult{}, fmt.Errorf("crash command, store opener, and visibility inspector are required")
	}
	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Env = append(os.Environ(), command.Env...)
	err := cmd.Run()
	result := CrashTrialResult{}
	if err != nil {
		result.ExitError = err.Error()
		result.WorkerCrashed = true
	}
	store, closeStore, openErr := open()
	if openErr != nil {
		return result, fmt.Errorf("reopen crash backend: %w", openErr)
	}
	if closeStore != nil {
		defer closeStore()
	}
	outcome, inspectErr := inspect(ctx, store)
	if inspectErr != nil {
		return result, inspectErr
	}
	result.Outcome = outcome
	return result, nil
}

// RunCrashCampaign repeats a failpoint on independently prepared durable
// stores. Reusing one store would let an earlier APPLIED trial contaminate the
// classification of later trials, so the plan must provide each trial's
// command, opener and unique intent.
func RunCrashCampaign(ctx context.Context, trials int, plan CrashTrialPlan) (CrashCampaignResult, error) {
	if trials < MinCrashCampaignTrials {
		return CrashCampaignResult{}, fmt.Errorf("crash campaign requires at least %d trials, got %d", MinCrashCampaignTrials, trials)
	}
	if plan == nil {
		return CrashCampaignResult{}, fmt.Errorf("crash trial plan is required")
	}
	result := CrashCampaignResult{Trials: make([]CrashTrialResult, 0, trials), Passed: true}
	for index := 0; index < trials; index++ {
		command, open, intent, err := plan(index)
		if err != nil {
			return result, fmt.Errorf("prepare crash trial %d: %w", index, err)
		}
		trial, err := RunCrashTrial(ctx, command, open, intent)
		if err != nil {
			return result, fmt.Errorf("run crash trial %d: %w", index, err)
		}
		result.Trials = append(result.Trials, trial)
		switch trial.Outcome {
		case OutcomeNotApplied:
			result.Counts.NotApplied++
		case OutcomeApplied:
			result.Counts.Applied++
		case OutcomeInvalidPartial:
			result.Counts.InvalidPartial++
		default:
			return result, fmt.Errorf("crash trial %d returned unknown outcome %q", index, trial.Outcome)
		}
		if !trial.WorkerCrashed || trial.Outcome == OutcomeInvalidPartial {
			result.Passed = false
		}
	}
	return result, nil
}
