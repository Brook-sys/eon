// Package fakeserver provides a deterministic OpenAI-compatible Chat
// Completions server for offline contract and vertical-slice tests.
package fakeserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
)

type Exchange struct {
	ExpectedPrompt         string
	ExpectedModel          string
	ExpectedMaxOutputField string
	// ExpectedResponseFormat is the response_format.type value when set
	// (for example "json_object"). Empty means the field must be absent.
	ExpectedResponseFormat string
	// RequireResponseFormat when true fails if response_format is missing.
	RequireResponseFormat bool
	ResponseText          string
	ResponseModel         string
	InputTokens           int
	OutputTokens          int
	StatusCode            int
	RawBody               string
}

type Request struct {
	Prompt          string
	Model           string
	MaxOutputTokens int
	MaxOutputField  string
	Temperature     float64
	Authorization   string
	ResponseFormat  string
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
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || len(body.Messages) != 1 || body.Messages[0].Role != "user" {
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
	status := exchange.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if exchange.RawBody != "" {
		_, _ = w.Write([]byte(exchange.RawBody))
		return
	}
	model := exchange.ResponseModel
	if model == "" {
		model = req.Model
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model":   model,
		"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": exchange.ResponseText}}},
		"usage":   map[string]any{"prompt_tokens": exchange.InputTokens, "completion_tokens": exchange.OutputTokens},
	})
}
