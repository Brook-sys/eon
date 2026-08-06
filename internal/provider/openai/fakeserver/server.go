// Package fakeserver provides a deterministic OpenAI-compatible Chat
// Completions server for offline contract and vertical-slice tests.
package fakeserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

const maxRequestBytes int64 = 1 << 20

type Exchange struct {
	ExpectedPrompt         string
	ExpectedModel          string
	ExpectedMaxOutputField string
	// ExpectedPrefillAssistant is the trailing assistant-role content the caller
	// must have sent (for adapter prefill contract tests). Empty means no
	// prefill message is expected (legacy single-user-message strictness).
	ExpectedPrefillAssistant string
	// ExpectedResponseFormat is the response_format.type value when set
	// (for example "json_object"). Empty means the field must be absent.
	ExpectedResponseFormat string
	// RequireResponseFormat when true fails if response_format is missing.
	RequireResponseFormat bool
	// ExpectedReasoningEffort is the reasoning_effort value expected in the
	// request body. Empty means the field must be absent (baseline behavior).
	ExpectedReasoningEffort string
	ResponseText            string
	ResponseModel           string
	ModelsResponse          []string
	InputTokens             int
	OutputTokens            int
	StatusCode              int
	RawBody                 string
	Headers                 map[string]string
	FinishReason            string
	ToolCalls               []struct {
		Name      string
		Arguments string
	}
}

type Request struct {
	Prompt          string
	Model           string
	MaxOutputTokens int
	MaxOutputField  string
	Temperature     float64
	Authorization   string
	ResponseFormat  string
	ReasoningEffort string
	UserAgent       string
	// PrefillAssistant captures the trailing assistant message content when the
	// caller appended one (adapter prefill contract evidence).
	PrefillAssistant string
	ToolCalls        []struct {
		Name      string
		Arguments string
	}
}

type Server struct {
	server   *httptest.Server
	mu       sync.Mutex
	script   []Exchange
	requests []Request
	failures []string
}

func New(script ...Exchange) *Server {
	s := &Server{script: append([]Exchange(nil), script...)}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *Server) URL() string          { return s.server.URL }
func (s *Server) Client() *http.Client { return s.server.Client() }
func (s *Server) Close()               { s.server.Close() }

func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func (s *Server) Failures() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.failures...)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		s.handleModels(w, r)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		s.failures = append(s.failures, fmt.Sprintf("unexpected request %s %s", r.Method, r.URL.Path))
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var body struct {
		Model               string                           `json:"model"`
		Messages            []struct{ Role, Content string } `json:"messages"`
		MaxTokens           int                              `json:"max_tokens"`
		MaxCompletionTokens int                              `json:"max_completion_tokens"`
		Temperature         float64                          `json:"temperature"`
		ResponseFormat      *struct {
			Type string `json:"type"`
		} `json:"response_format"`
		ReasoningEffort string `json:"reasoning_effort"`
		Tools           []struct {
			Type     string `json:"type"`
			Function struct {
				Name       string          `json:"name"`
				Parameters json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	decodeErr := decoder.Decode(&body)
	trailingErr := error(nil)
	if decodeErr == nil {
		trailingErr = decoder.Decode(&struct{}{})
	}
	if decodeErr != nil || trailingErr != io.EOF || len(body.Messages) == 0 || body.Messages[0].Role != "user" {
		// Keep the externally asserted failure stable and free of request/body
		// details. Tests need deterministic classification, not decoder text.
		s.failures = append(s.failures, "invalid Chat Completions request")
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	prefill := ""
	switch len(body.Messages) {
	case 1:
		// baseline single user message
	case 2:
		if body.Messages[1].Role != "assistant" {
			s.failures = append(s.failures, "invalid Chat Completions request")
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		prefill = body.Messages[1].Content
	default:
		s.failures = append(s.failures, "invalid Chat Completions request")
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	maxOutputField := ""
	maxOutputTokens := 0
	if body.MaxTokens != 0 {
		maxOutputField, maxOutputTokens = "max_tokens", body.MaxTokens
	}
	if body.MaxCompletionTokens != 0 {
		if maxOutputField != "" {
			s.failures = append(s.failures, "both max output fields were sent")
		}
		maxOutputField, maxOutputTokens = "max_completion_tokens", body.MaxCompletionTokens
	}
	responseFormat := ""
	if body.ResponseFormat != nil {
		responseFormat = body.ResponseFormat.Type
	}
	req := Request{
		Prompt: body.Messages[0].Content, Model: body.Model, MaxOutputTokens: maxOutputTokens,
		MaxOutputField: maxOutputField, Temperature: body.Temperature,
		Authorization: r.Header.Get("Authorization"), ResponseFormat: responseFormat,
		ReasoningEffort: body.ReasoningEffort, UserAgent: r.Header.Get("User-Agent"),
		PrefillAssistant: prefill,
	}
	s.requests = append(s.requests, req)
	if len(s.script) == 0 {
		s.failures = append(s.failures, "unexpected extra request")
		http.Error(w, "script exhausted", http.StatusInternalServerError)
		return
	}
	exchange := s.script[0]
	s.script = s.script[1:]
	if exchange.ExpectedPrompt != "" && exchange.ExpectedPrompt != req.Prompt {
		s.failures = append(s.failures, fmt.Sprintf("prompt mismatch: got %q", req.Prompt))
	}
	if exchange.ExpectedModel != "" && exchange.ExpectedModel != req.Model {
		s.failures = append(s.failures, fmt.Sprintf("model mismatch: got %q", req.Model))
	}
	if exchange.ExpectedMaxOutputField != "" && exchange.ExpectedMaxOutputField != req.MaxOutputField {
		s.failures = append(s.failures, fmt.Sprintf("max output field mismatch: got %q", req.MaxOutputField))
	}
	if exchange.ExpectedPrefillAssistant != "" && exchange.ExpectedPrefillAssistant != req.PrefillAssistant {
		s.failures = append(s.failures, fmt.Sprintf("prefill assistant mismatch: got %q", req.PrefillAssistant))
	}
	if exchange.ExpectedPrefillAssistant == "" && req.PrefillAssistant != "" {
		s.failures = append(s.failures, fmt.Sprintf("unexpected prefill assistant %q", req.PrefillAssistant))
	}
	if exchange.RequireResponseFormat && req.ResponseFormat == "" {
		s.failures = append(s.failures, "expected response_format, got none")
	}
	if exchange.ExpectedResponseFormat != "" && exchange.ExpectedResponseFormat != req.ResponseFormat {
		s.failures = append(s.failures, fmt.Sprintf("response_format mismatch: got %q", req.ResponseFormat))
	}
	if exchange.ExpectedResponseFormat == "" && !exchange.RequireResponseFormat && req.ResponseFormat != "" {
		// Scripts that do not opt into response_format must remain baseline.
		s.failures = append(s.failures, fmt.Sprintf("unexpected response_format %q", req.ResponseFormat))
	}
	if exchange.ExpectedReasoningEffort != "" && exchange.ExpectedReasoningEffort != req.ReasoningEffort {
		s.failures = append(s.failures, fmt.Sprintf("reasoning_effort mismatch: expected %q, got %q", exchange.ExpectedReasoningEffort, req.ReasoningEffort))
	}
	if exchange.ExpectedReasoningEffort == "" && req.ReasoningEffort != "" {
		s.failures = append(s.failures, fmt.Sprintf("unexpected reasoning_effort %q", req.ReasoningEffort))
	}
	status := exchange.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	if len(exchange.Headers) > 0 {
		for k, v := range exchange.Headers {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(status)
	if exchange.RawBody != "" {
		_, _ = w.Write([]byte(exchange.RawBody))
		return
	}
	model := exchange.ResponseModel
	if model == "" {
		model = req.Model
	}

	message := map[string]any{"role": "assistant", "content": exchange.ResponseText}
	finishReason := exchange.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}
	if len(exchange.ToolCalls) > 0 {
		if exchange.FinishReason == "" {
			finishReason = "tool_calls"
		}
		var tcList []any
		for i, tc := range exchange.ToolCalls {
			tcList = append(tcList, map[string]any{
				"id":   fmt.Sprintf("call_%d", i),
				"type": "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			})
		}
		message["tool_calls"] = tcList
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"model":   model,
		"choices": []any{map[string]any{"message": message, "finish_reason": finishReason}},
		"usage":   map[string]any{"prompt_tokens": exchange.InputTokens, "completion_tokens": exchange.OutputTokens},
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if len(s.script) == 0 {
		s.failures = append(s.failures, "unexpected extra request for models")
		http.Error(w, "script exhausted", http.StatusInternalServerError)
		return
	}
	exchange := s.script[0]
	s.script = s.script[1:]
	status := exchange.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	for key, value := range exchange.Headers {
		w.Header().Set(key, value)
	}
	w.WriteHeader(status)
	if exchange.RawBody != "" {
		_, _ = w.Write([]byte(exchange.RawBody))
		return
	}
	data := make([]map[string]string, len(exchange.ModelsResponse))
	for i, id := range exchange.ModelsResponse {
		data[i] = map[string]string{"id": id}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}
