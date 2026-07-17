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
	"strings"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const (
	defaultMaxResponseBytes int64 = 1 << 20
	defaultHTTPTimeout            = 2 * time.Minute
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
	// probeBudget is the remaining live Probe allowance (FR-MODEL-005).
	mu          sync.Mutex
	probeBudget int
	lastProbe   domain.ProviderProfile
	hasProbe    bool
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
	// RetryAfter is the earliest safe retry delay declared by the provider.
	// It is parsed only from the standard Retry-After header; response bodies
	// remain discarded because they may echo prompts or credentials.
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("openai-compatible provider: %s (status %d)", e.Kind, e.StatusCode)
	}
	return fmt.Sprintf("openai-compatible provider: %s", e.Kind)
}

// RetryAfterDelay exposes bounded provider backpressure without coupling the
// kernel to this adapter's concrete error type.
func (e *Error) RetryAfterDelay() time.Duration { return e.RetryAfter }

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
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/chat/completions"
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
		probeBudget:      1,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
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
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *Provider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	if strings.TrimSpace(request.Prompt) == "" || request.MaxOutputTokens < 0 || request.Temperature < 0 || request.Temperature > 2 {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	chatReq := chatRequest{Model: p.model, Messages: []chatMessage{{Role: "user", Content: request.Prompt}}, Temperature: request.Temperature}
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
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded.Choices) != 1 || decoded.Choices[0].Message.Role != "assistant" || decoded.Choices[0].Message.Content == "" || decoded.Usage.PromptTokens < 0 || decoded.Usage.CompletionTokens < 0 {
		return port.CompletionResult{}, &Error{Kind: ErrorInvalidResponse}
	}
	return port.CompletionResult{Text: decoded.Choices[0].Message.Content, InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens, Model: decoded.Model}, nil
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
)
