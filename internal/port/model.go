package port

import (
	"context"

	"motor-autonomo/internal/domain"
)

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

// ModelProvider is the minimum text→text contract. Adapters may also implement
// ModelCapabilityReporter for FR-MODEL-005 discovery without expanding this
// surface for every test double.
type ModelProvider interface {
	Complete(context.Context, CompletionRequest) (CompletionResult, error)
}

// ModelCapabilityReporter exposes versioned provider/model capability snapshots.
// DeclaredProfile must not perform network I/O. Probe may perform a budgeted,
// cacheable check and MUST NOT loop or invent support for unknown features.
type ModelCapabilityReporter interface {
	DeclaredProfile() domain.ProviderProfile
	Probe(context.Context) (domain.ProviderProfile, error)
}
