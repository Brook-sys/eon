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
	// PrefillAssistant is an optional opening fragment for the assistant reply.
	// When non-empty, adapters that support provider prefill (e.g. Groq/NIM
	// chat-completions) emit a trailing assistant message with this exact
	// content so the model is forced to continue from it. V2 of the Phase 371
	// prompt-variations campaign showed this is the only variation that
	// eliminates Markdown fences on llama-3.1-8b-instant without trading strict
	// JSON validity, and it carries no semantic answer when the prefix is a
	// pure structural opener like "{" or "[".
	PrefillAssistant string
}

type CompletionResult struct {
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
	Model        string
	// FinishReason is a bounded provider-neutral classification. Adapters must
	// map unknown or absent wire values instead of retaining arbitrary text.
	FinishReason CompletionFinishReason
}

type CompletionFinishReason string

const (
	CompletionFinishUnknown       CompletionFinishReason = "unknown"
	CompletionFinishStop          CompletionFinishReason = "stop"
	CompletionFinishLength        CompletionFinishReason = "length"
	CompletionFinishToolCalls     CompletionFinishReason = "tool_calls"
	CompletionFinishContentFilter CompletionFinishReason = "content_filter"
	CompletionFinishOther         CompletionFinishReason = "other"
)

// ToolCall represents a model's request to execute a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// DurableModelCompletionResult converts the complete provider-neutral result
// into domain-owned persistence data without making domain depend on port.
func DurableModelCompletionResult(result CompletionResult) domain.ModelCompletionResult {
	var toolCalls []domain.ModelCompletionToolCall
	if result.ToolCalls != nil {
		toolCalls = make([]domain.ModelCompletionToolCall, len(result.ToolCalls))
	}
	for i, call := range result.ToolCalls {
		args := call.Arguments
		if len(args) > 65536 {
			args = args[:65536] + "... (truncated)"
		}
		toolCalls[i] = domain.ModelCompletionToolCall{ID: call.ID, Name: call.Name, Arguments: args}
	}
	return domain.ModelCompletionResult{
		Text: result.Text, ToolCalls: toolCalls, InputTokens: result.InputTokens,
		OutputTokens: result.OutputTokens, Model: result.Model, FinishReason: string(result.FinishReason),
	}
}

// CompletionResultFromDurable reconstructs the complete provider-neutral
// result from a durable receipt payload.
func CompletionResultFromDurable(result domain.ModelCompletionResult) CompletionResult {
	var toolCalls []ToolCall
	if result.ToolCalls != nil {
		toolCalls = make([]ToolCall, len(result.ToolCalls))
	}
	for i, call := range result.ToolCalls {
		toolCalls[i] = ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments}
	}
	return CompletionResult{
		Text: result.Text, ToolCalls: toolCalls, InputTokens: result.InputTokens,
		OutputTokens: result.OutputTokens, Model: result.Model, FinishReason: CompletionFinishReason(result.FinishReason),
	}
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

// ProviderDiagnosticError is the optional adapter-neutral projection for
// structured, non-sensitive diagnostic labels (e.g. which validation
// condition triggered INVALID_RESPONSE). It never exposes response bodies,
// headers, or any other potentially sensitive payload — only short labels
// that classify the failure mode.
type ProviderDiagnosticError interface {
	ProviderError
	DiagnosticReason() string
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

// ModelToolProvider extends ModelProvider with support for tool definitions
// (functions) during completion requests.
type ModelToolProvider interface {
	ModelProvider
	CompleteWithTools(context.Context, CompletionRequest, []ToolDefinition) (CompletionResult, error)
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
