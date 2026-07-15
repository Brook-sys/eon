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

	"motor-autonomo/internal/port"
)

const defaultMaxResponseBytes int64 = 1 << 20

type Config struct {
	BaseURL          string
	APIKey           string
	Model            string
	MaxResponseBytes int64
	Client           *http.Client
}

type Provider struct {
	endpoint         string
	apiKey           string
	model            string
	maxResponseBytes int64
	client           *http.Client
}

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
}

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("openai-compatible provider: %s (status %d)", e.Kind, e.StatusCode)
	}
	return fmt.Sprintf("openai-compatible provider: %s", e.Kind)
}

func New(config Config) (*Provider, error) {
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
		client = http.DefaultClient
	}
	return &Provider{endpoint: base.String(), apiKey: config.APIKey, model: config.Model, maxResponseBytes: limit, client: client}, nil
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
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
	payload, err := json.Marshal(chatRequest{Model: p.model, Messages: []chatMessage{{Role: "user", Content: request.Prompt}}, MaxTokens: request.MaxOutputTokens, Temperature: request.Temperature})
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
		return port.CompletionResult{}, &Error{Kind: ErrorHTTP, StatusCode: response.StatusCode, Retryable: response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500}
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
