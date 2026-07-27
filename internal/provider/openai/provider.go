// Package openai adapts the provider-neutral text completion port to the
// OpenAI-compatible Chat Completions HTTP dialect.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const (
	defaultMaxResponseBytes int64 = 1 << 20
	defaultHTTPTimeout            = 2 * time.Minute
	defaultModelsCacheTTL         = 5 * time.Minute
	maxDiscoveredModels           = 100
)

type Config struct {
	BaseURL          string
	APIKey           string
	Model            string
	MaxOutputField   MaxOutputField
	MaxResponseBytes int64
	Timeout          time.Duration
	Client           *http.Client
}

type Provider struct {
	endpoint         string
	apiKey           string
	model            string
	maxOutputField   MaxOutputField
	maxResponseBytes int64
	client           *http.Client
	// name is the operator-facing profile label (never a secret).
	name string
	// contextTokens is the declared context window for budgeting (0 = unknown).
	contextTokens int
	// allowedModels is an optional allowlist for discovery. Empty means all are allowed.
	allowedModels  []string
	modelsCacheTTL time.Duration
	// probeBudget is the remaining live Probe allowance (FR-MODEL-005).
	mu           sync.Mutex
	probeBudget  int
	lastProbe    domain.ProviderProfile
	hasProbe     bool
	cachedModels []string
	modelsTTL    time.Time
}

// Option configures optional non-secret provider metadata.
type Option func(*Provider)

// WithProfileName sets the declared profile name (default openai-compatible).
func WithProfileName(name string) Option {
	return func(p *Provider) {
		if strings.TrimSpace(name) != "" {
			p.name = strings.TrimSpace(name)
		}
	}
}

// WithContextTokens records the declared context window for budgeting.
func WithContextTokens(n int) Option {
	return func(p *Provider) {
		if n > 0 {
			p.contextTokens = n
		}
	}
}

// WithProbeBudget sets how many live Probe calls are allowed (default 1).
func WithProbeBudget(n int) Option {
	return func(p *Provider) {
		if n >= 0 {
			p.probeBudget = n
		}
	}
}

// WithAllowedModels replaces the discovery allowlist. Empty IDs are ignored;
// an empty resulting list falls back to the configured model only.
func WithAllowedModels(models ...string) Option {
	return func(p *Provider) {
		p.allowedModels = normalizeModelIDs(models, 0)
	}
}

// WithModelsCacheTTL configures successful discovery caching. Non-positive
// values are ignored; production defaults to five minutes.
func WithModelsCacheTTL(ttl time.Duration) Option {
	return func(p *Provider) {
		if ttl > 0 {
			p.modelsCacheTTL = ttl
		}
	}
}

// MaxOutputField identifies the incompatible request field used to bound a
// Chat Completions response. OpenAI-compatible servers do not agree on one
// spelling, so the adapter requires an explicit profile when the portable
// legacy field is not suitable.
type MaxOutputField string

const (
	MaxOutputTokensLegacy     MaxOutputField = "max_tokens"
	MaxOutputTokensCompletion MaxOutputField = "max_completion_tokens"
)

type ErrorKind string

const (
	ErrorInvalidRequest   ErrorKind = "INVALID_REQUEST"
	ErrorTransport        ErrorKind = "TRANSPORT"
	ErrorHTTP             ErrorKind = "HTTP"
	ErrorResponseTooLarge ErrorKind = "RESPONSE_TOO_LARGE"
	ErrorInvalidResponse  ErrorKind = "INVALID_RESPONSE"
)

// Error contains only bounded, non-secret adapter diagnostics. Response bodies
// are deliberately excluded because compatible servers can echo prompts or
// credentials in errors (FR-OBS-002).
type Error struct {
	Kind       ErrorKind
	StatusCode int
	Retryable  bool
	// Reason is a short, non-sensitive diagnostic label that distinguishes
	// which validation condition triggered INVALID_RESPONSE (e.g.
	// "json_unmarshal_failed", "choices_count", "role_not_assistant",
	// "empty_content", "negative_usage"). It never contains response body
	// text or any other potentially sensitive payload.
	Reason string
	// RetryAfter is the earliest safe retry delay declared by the provider.
	// It is parsed only from the standard Retry-After header; response bodies
	// remain discarded because they may echo prompts or credentials.
	RetryAfter time.Duration
	// RateLimit contains only parsed, allowlisted quota fields. Unknown headers
	// and raw values are never retained or exposed.
	RateLimit port.RateLimitMetadata
}

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		if e.Reason != "" {
			return fmt.Sprintf("openai-compatible provider: %s (status %d): %s", e.Kind, e.StatusCode, e.Reason)
		}
		return fmt.Sprintf("openai-compatible provider: %s (status %d)", e.Kind, e.StatusCode)
	}
	if e.Reason != "" {
		return fmt.Sprintf("openai-compatible provider: %s: %s", e.Kind, e.Reason)
	}
	return fmt.Sprintf("openai-compatible provider: %s", e.Kind)
}

// RetryAfterDelay exposes bounded provider backpressure without coupling the
// kernel to this adapter's concrete error type.
func (e *Error) RetryAfterDelay() time.Duration { return e.RetryAfter }

func (e *Error) RateLimitMetadata() port.RateLimitMetadata { return e.RateLimit }

func (e *Error) HTTPStatusCode() int    { return e.StatusCode }
func (e *Error) RetryableFailure() bool { return e.Retryable }

// DiagnosticReason returns a short, non-sensitive label that classifies which
// validation condition triggered the error (e.g. "json_unmarshal_failed",
// "empty_content"). It never contains response body text.
func (e *Error) DiagnosticReason() string { return e.Reason }

// New creates an OpenAI-compatible chat completions adapter. Optional Option
// values configure non-secret profile metadata used by DeclaredProfile/Probe.
func New(config Config, opts ...Option) (*Provider, error) {
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("base URL and model are required")
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, errors.New("base URL must be an absolute HTTP(S) URL")
	}
	base.Path = strings.TrimRight(base.Path, "/")
	if !strings.HasSuffix(base.Path, "/v1") {
		base.Path += "/v1"
	}
	base.Path += "/chat/completions"
	base.RawQuery = ""
	base.Fragment = ""
	limit := config.MaxResponseBytes
	if limit == 0 {
		limit = defaultMaxResponseBytes
	}
	if limit < 1 {
		return nil, errors.New("max response bytes must be positive")
	}
	client := config.Client
	if client == nil {
		timeout := config.Timeout
		if timeout == 0 {
			timeout = defaultHTTPTimeout
		}
		if timeout < 0 || timeout > 10*time.Minute {
			return nil, errors.New("HTTP timeout must be positive and at most ten minutes")
		}
		client = &http.Client{Timeout: timeout}
	} else if config.Timeout != 0 {
		return nil, errors.New("HTTP timeout cannot be combined with a custom client")
	}
	maxOutputField := config.MaxOutputField
	if maxOutputField == "" {
		maxOutputField = MaxOutputTokensLegacy
	}
	if maxOutputField != MaxOutputTokensLegacy && maxOutputField != MaxOutputTokensCompletion {
		return nil, errors.New("unsupported max output field")
	}
	p := &Provider{
		endpoint:         base.String(),
		apiKey:           config.APIKey,
		model:            config.Model,
		maxOutputField:   maxOutputField,
		maxResponseBytes: limit,
		client:           client,
		name:             "openai-compatible",
		allowedModels:    []string{strings.TrimSpace(config.Model)},
		modelsCacheTTL:   defaultModelsCacheTTL,
		probeBudget:      1,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	if len(p.allowedModels) == 0 {
		p.allowedModels = []string{p.model}
	}
	return p, nil
}

type chatRequest struct {
	Model               string          `json:"model"`
	Messages            []chatMessage   `json:"messages"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         float64         `json:"temperature"`
	ResponseFormat      *responseFormat `json:"response_format,omitempty"`
	Tools               []chatTool      `json:"tools,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *Provider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	return p.complete(ctx, request, nil)
}

// CompleteWithTools sends a bounded kernel-owned tool catalog using the
// OpenAI-compatible functions wire format. Tool execution remains outside the
// adapter; this method only transports definitions.
func (p *Provider) CompleteWithTools(ctx context.Context, request port.CompletionRequest, definitions []port.ToolDefinition) (port.CompletionResult, error) {
	tools := make([]chatTool, 0, len(definitions))
	for _, definition := range definitions {
		if strings.TrimSpace(definition.Name) == "" || len(definition.Parameters) == 0 || !json.Valid(definition.Parameters) {
			return port.CompletionResult{}, &Error{Kind: ErrorInvalidRequest}
		}
		tools = append(tools, chatTool{Type: "function", Function: chatFunction{
			Name: definition.Name, Description: definition.Description,
			Parameters: append(json.RawMessage(nil), definition.Parameters...),
		}})
	}
	return p.complete(ctx, request, tools)
}

func (p *Provider) complete(ctx context.Context, request port.CompletionRequest, tools []chatTool) (port.CompletionResult, error) {
	if strings.TrimSpace(request.Prompt) == "" || request.MaxOutputTokens < 0 || request.Temperature < 0 || request.Temperature > 2 {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	chatReq := chatRequest{Model: p.model, Messages: []chatMessage{{Role: "user", Content: request.Prompt}}, Temperature: request.Temperature, Tools: tools}
	if p.maxOutputField == MaxOutputTokensCompletion {
		chatReq.MaxCompletionTokens = request.MaxOutputTokens
	} else {
		chatReq.MaxTokens = request.MaxOutputTokens
	}
	// Optional FR-MODEL-006 enrichment: only emit response_format when the
	// kernel selected a known hint. Unknown values fail closed as invalid request.
	switch request.ResponseFormat {
	case domain.ResponseFormatNone:
		// baseline text→text
	case domain.ResponseFormatJSONObject:
		chatReq.ResponseFormat = &responseFormat{Type: "json_object"}
	default:
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	payload, err := json.Marshal(chatReq)
	if err != nil {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return port.CompletionResult{}, &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, p.maxResponseBytes+1))
		return port.CompletionResult{}, &Error{
			Kind:       ErrorHTTP,
			StatusCode: response.StatusCode,
			Retryable:  response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
			RateLimit:  parseRateLimitMetadata(response.Header),
		}
	}
	limited := io.LimitReader(response.Body, p.maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return port.CompletionResult{}, &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(body)) > p.maxResponseBytes {
		return port.CompletionResult{}, &Error{Kind: ErrorResponseTooLarge}
	}
	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidResponse, Reason: "json_unmarshal_failed"}
	}
	if len(decoded.Choices) != 1 {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidResponse, Reason: "choices_count"}
	}
	if decoded.Choices[0].Message.Role != "assistant" {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidResponse, Reason: "role_not_assistant"}
	}
	if decoded.Choices[0].Message.Content == "" && len(decoded.Choices[0].Message.ToolCalls) == 0 {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidResponse, Reason: "empty_content"}
	}
	if decoded.Usage.PromptTokens < 0 || decoded.Usage.CompletionTokens < 0 {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidResponse, Reason: "negative_usage"}
	}

	var toolCalls []port.ToolCall
	for _, call := range decoded.Choices[0].Message.ToolCalls {
		if call.Type == "function" {
			toolCalls = append(toolCalls, port.ToolCall{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			})
		}
	}

	return port.CompletionResult{Text: decoded.Choices[0].Message.Content, ToolCalls: toolCalls, InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens, Model: decoded.Model, FinishReason: classifyFinishReason(decoded.Choices[0].FinishReason)}, nil
}

func classifyFinishReason(value string) port.CompletionFinishReason {
	switch strings.TrimSpace(value) {
	case "stop":
		return port.CompletionFinishStop
	case "length":
		return port.CompletionFinishLength
	case "tool_calls", "function_call":
		return port.CompletionFinishToolCalls
	case "content_filter":
		return port.CompletionFinishContentFilter
	case "":
		return port.CompletionFinishUnknown
	default:
		return port.CompletionFinishOther
	}
}

// parseRetryAfter accepts both forms from RFC 9110: delay-seconds and an HTTP
// date. Invalid, negative, or already elapsed values fail closed to zero so the
// deterministic ResourceGate backoff remains the fallback.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := time.ParseDuration(value + "s"); err == nil {
		if seconds > 0 {
			return seconds
		}
		return 0
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}

func parseRateLimitMetadata(header http.Header) port.RateLimitMetadata {
	var metadata port.RateLimitMetadata
	metadata.RequestLimit, metadata.HasRequestLimit = parseNonNegativeInt64(header.Get("x-ratelimit-limit-requests"))
	metadata.RequestRemaining, metadata.HasRequestRemaining = parseNonNegativeInt64(header.Get("x-ratelimit-remaining-requests"))
	metadata.RequestReset, metadata.HasRequestReset = parseNonNegativeDuration(header.Get("x-ratelimit-reset-requests"))
	metadata.TokenLimit, metadata.HasTokenLimit = parseNonNegativeInt64(header.Get("x-ratelimit-limit-tokens"))
	metadata.TokenRemaining, metadata.HasTokenRemaining = parseNonNegativeInt64(header.Get("x-ratelimit-remaining-tokens"))
	metadata.TokenReset, metadata.HasTokenReset = parseNonNegativeDuration(header.Get("x-ratelimit-reset-tokens"))
	return metadata
}

func parseNonNegativeInt64(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 20 {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

func parseNonNegativeDuration(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 32 {
		return 0, false
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

// DeclaredProfile returns the conservative configuration snapshot without I/O.
// Richer features stay false until a successful Probe or operator override.
func (p *Provider) DeclaredProfile() domain.ProviderProfile {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.declaredLocked(time.Now().UTC())
}

func (p *Provider) declaredLocked(now time.Time) domain.ProviderProfile {
	dialect := domain.MaxOutputDialectLegacy
	if p.maxOutputField == MaxOutputTokensCompletion {
		dialect = domain.MaxOutputDialectCompletion
	}
	profile := domain.BaselineDeclaredProfile(p.name, p.model, dialect, p.contextTokens, now)
	profile.ProbeBudgetRemaining = p.probeBudget
	return profile
}

// Probe performs at most the remaining budget of live text→text checks. When
// the budget is exhausted, the last probe (or declared baseline) is returned
// without network I/O so probes cannot form an autodetection loop.
func (p *Provider) Probe(ctx context.Context) (domain.ProviderProfile, error) {
	p.mu.Lock()
	if p.hasProbe && p.probeBudget <= 0 {
		cached := p.lastProbe
		cached.ProbeBudgetRemaining = 0
		p.mu.Unlock()
		return cached, nil
	}
	if p.probeBudget <= 0 {
		// No successful probe yet; surface declared snapshot without I/O.
		profile := p.declaredLocked(time.Now().UTC())
		profile.ProbeBudgetRemaining = 0
		profile.SafeDetail = "probe budget exhausted; returning declared baseline without network I/O"
		p.mu.Unlock()
		return profile, nil
	}
	// Consume one unit before the call so concurrent/racy probes cannot loop.
	p.probeBudget--
	remaining := p.probeBudget
	p.mu.Unlock()

	result, err := p.Complete(ctx, port.CompletionRequest{
		Prompt:          "ping",
		MaxOutputTokens: 1,
		Temperature:     0,
	})
	now := time.Now().UTC()
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		profile := p.declaredLocked(now)
		profile.TextToTextConfirmed = false
		profile.Source = domain.CapabilityProbed
		profile.ProbeBudgetRemaining = remaining
		profile.SafeDetail = fmt.Sprintf("text-to-text probe failed: %s", classifyProbeError(err))
		// Do not cache a failed probe as confirmation; still record for budget.
		p.lastProbe = profile
		p.hasProbe = true
		return profile, nil
	}
	profile := p.declaredLocked(now)
	profile.TextToTextConfirmed = true
	profile.Source = domain.CapabilityProbed
	profile.ProbeBudgetRemaining = remaining
	if result.Model != "" {
		profile.Model = result.Model
	}
	profile.SafeDetail = "text-to-text chat completions probe succeeded; richer capabilities remain unconfirmed"
	p.lastProbe = profile
	p.hasProbe = true
	return profile, nil
}

// DiscoverModels fetches available models from the optional /v1/models endpoint,
// caching the result for the configured TTL to avoid rate limit pressure.
func (p *Provider) DiscoverModels(ctx context.Context) ([]string, error) {
	p.mu.Lock()
	if time.Now().Before(p.modelsTTL) {
		cached := make([]string, len(p.cachedModels))
		copy(cached, p.cachedModels)
		p.mu.Unlock()
		return cached, nil
	}
	p.mu.Unlock()

	endpoint := strings.TrimSuffix(p.endpoint, "/chat/completions") + "/models"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidRequest}
	}
	if p.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, p.maxResponseBytes+1))
		return nil, &Error{
			Kind:       ErrorHTTP,
			StatusCode: response.StatusCode,
			Retryable:  response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
		}
	}

	limited := io.LimitReader(response.Body, p.maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(body)) > p.maxResponseBytes {
		return nil, &Error{Kind: ErrorResponseTooLarge}
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &Error{Kind: ErrorInvalidResponse, Reason: "models_json_unmarshal_failed"}
	}
	if payload.Data == nil {
		return nil, &Error{Kind: ErrorInvalidResponse, Reason: "models_data_nil"}
	}

	allowset := make(map[string]struct{}, len(p.allowedModels))
	for _, id := range p.allowedModels {
		allowset[id] = struct{}{}
	}
	rawIDs := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if _, ok := allowset[id]; ok {
			rawIDs = append(rawIDs, id)
		}
	}
	allowed := normalizeModelIDs(rawIDs, maxDiscoveredModels)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.cachedModels = allowed
	p.modelsTTL = time.Now().Add(p.modelsCacheTTL)
	copied := make([]string, len(allowed))
	copy(copied, allowed)
	return copied, nil
}

func normalizeModelIDs(raw []string, limit int) []string {
	seen := make(map[string]struct{}, len(raw))
	normalized := make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			normalized = append(normalized, id)
			if limit > 0 && len(normalized) == limit {
				break
			}
		}
	}
	return normalized
}

func classifyProbeError(err error) string {
	var providerError *Error
	if errors.As(err, &providerError) {
		if providerError.StatusCode != 0 {
			return fmt.Sprintf("%s status=%d", providerError.Kind, providerError.StatusCode)
		}
		return string(providerError.Kind)
	}
	return "unclassified"
}

// Ensure Provider satisfies the capability reporter surface used by inspect.
var (
	_ port.ModelProvider           = (*Provider)(nil)
	_ port.ModelCapabilityReporter = (*Provider)(nil)
	_ port.ModelDiscoveryReporter  = (*Provider)(nil)
)
