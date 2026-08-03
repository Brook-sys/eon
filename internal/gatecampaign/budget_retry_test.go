package gatecampaign

import (
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/port"
)

// stubDiagnosticError implements port.ProviderDiagnosticError for tests.
type stubDiagnosticError struct {
	reason  string
	message string
}

var _ port.ProviderDiagnosticError = stubDiagnosticError{}

func (e stubDiagnosticError) Error() string                  { return e.message }
func (e stubDiagnosticError) RetryAfterDelay() time.Duration { return 0 }
func (e stubDiagnosticError) DiagnosticReason() string       { return e.reason }

func TestShouldRetryWithHigherBudget(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		currentBudget int
		maxBudget     int
		wantRetry     bool
		wantBudget    int
	}{
		{
			name:          "reasoning exhausted scales by 4",
			err:           stubDiagnosticError{reason: ReasoningBudgetExhausted, message: "eaten"},
			currentBudget: 8,
			maxBudget:     1024,
			wantRetry:     true,
			wantBudget:    32,
		},
		{
			name:          "reasoning exhausted scales by 4 from 128",
			err:           stubDiagnosticError{reason: ReasoningBudgetExhausted, message: "eaten"},
			currentBudget: 128,
			maxBudget:     1024,
			wantRetry:     true,
			wantBudget:    512,
		},
		{
			name:          "scaled budget clamped to max",
			err:           stubDiagnosticError{reason: ReasoningBudgetExhausted, message: "eaten"},
			currentBudget: 512,
			maxBudget:     1024,
			wantRetry:     true,
			wantBudget:    1024,
		},
		{
			name:          "already at max no retry",
			err:           stubDiagnosticError{reason: ReasoningBudgetExhausted, message: "eaten"},
			currentBudget: 1024,
			maxBudget:     1024,
			wantRetry:     false,
			wantBudget:    1024,
		},
		{
			name:          "empty content no retry",
			err:           stubDiagnosticError{reason: "empty_content", message: "empty"},
			currentBudget: 8,
			maxBudget:     1024,
			wantRetry:     false,
			wantBudget:    8,
		},
		{
			name:          "nil error no retry",
			err:           nil,
			currentBudget: 64,
			maxBudget:     1024,
			wantRetry:     false,
			wantBudget:    64,
		},
		{
			name:          "non-diagnostic error no retry",
			err:           errors.New("some other failure"),
			currentBudget: 64,
			maxBudget:     1024,
			wantRetry:     false,
			wantBudget:    64,
		},
		{
			name:          "zero budget no retry",
			err:           stubDiagnosticError{reason: ReasoningBudgetExhausted, message: "eaten"},
			currentBudget: 0,
			maxBudget:     1024,
			wantRetry:     false,
			wantBudget:    0,
		},
		{
			name:          "no max budget cap allows scaling",
			err:           stubDiagnosticError{reason: ReasoningBudgetExhausted, message: "eaten"},
			currentBudget: 256,
			maxBudget:     0,
			wantRetry:     true,
			wantBudget:    1024,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			retry, newBudget := ShouldRetryWithHigherBudget(test.err, test.currentBudget, test.maxBudget)
			if retry != test.wantRetry {
				t.Fatalf("retry = %v, want %v", retry, test.wantRetry)
			}
			if newBudget != test.wantBudget {
				t.Fatalf("newBudget = %d, want %d", newBudget, test.wantBudget)
			}
		})
	}
}
