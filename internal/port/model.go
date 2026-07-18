package port

import (
	"context"
	"time"

	"motor-autonomo/internal/domain"
)

// CompletionRequest is the provider-neutral minimum model contract
// (FR-MODEL-001). Prompt and response are plain text; richer provider
// features must remain optional, authority-free adapter details selected by
// the kernel from a confirmed ProviderProfile (FR-MODEL-006).
type CompletionRequest struct {
	Prompt          string
	MaxOutputTokens int
	Temperature     float64
	// ResponseFormat is an optional enrichment (for example json_object).
	// Empty means baseline text→text. Adapters MUST ignore unknown values
	// safely or reject without inventing capabilities.
	ResponseFormat domain.ResponseFormatHint
}

type CompletionResult struct {
	Text         string
	InputTokens  int
	OutputTokens int
	Model        string
}

// ProviderError represents an active rejection from the provider.
// It exposes retry availability without exposing raw error bodies.
type ProviderError interface {
	error
	// RetryAfterDelay returns the earliest safe retry delay declared by the provider,
	// or zero if none was provided.
	RetryAfterDelay() time.Duration
}

// ProviderHTTPError is the optional adapter-neutral HTTP failure projection used
// by binding recovery policy. It deliberately excludes response bodies and
// headers other than the bounded Retry-After delay exposed by ProviderError.
type ProviderHTTPError interface {
	ProviderError
	HTTPStatusCode() int
	RetryableFailure() bool
}

// RateLimitMetadata is an allowlisted, provider-neutral projection of quota
// headers. Presence flags distinguish an observed zero (for example no calls
// remaining) from a header the provider did not send. Raw header names and
// values are deliberately not retained.
type RateLimitMetadata struct {
	HasRequestLimit     bool
	RequestLimit        int64
	HasRequestRemaining bool
	RequestRemaining    int64
	HasRequestReset     bool
	RequestReset        time.Duration
	HasTokenLimit       bool
	TokenLimit          int64
	HasTokenRemaining   bool
	TokenRemaining      int64
	HasTokenReset       bool
	TokenReset          time.Duration
}

// ProviderRateLimitError is an optional extension implemented by adapters
// that can safely project known quota headers without exposing arbitrary
// provider response metadata.
type ProviderRateLimitError interface {
	ProviderError
	RateLimitMetadata() RateLimitMetadata
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

// ModelDiscoveryReporter optionally lists provider-reported model IDs for
// operator inspection. Discovery is read-only and MUST NOT change routing,
// bindings, capability grants, or any other canonical authority automatically.
type ModelDiscoveryReporter interface {
	DiscoverModels(context.Context) ([]string, error)
}
