package port

import "context"

// CompletionRequest is the provider-neutral minimum model contract
// (FR-MODEL-001). Prompt and response are plain text; richer provider
// features must remain adapter details.
type CompletionRequest struct {
	Prompt          string
	MaxOutputTokens int
	Temperature     float64
}

type CompletionResult struct {
	Text         string
	InputTokens  int
	OutputTokens int
	Model        string
}

type ModelProvider interface {
	Complete(context.Context, CompletionRequest) (CompletionResult, error)
}
