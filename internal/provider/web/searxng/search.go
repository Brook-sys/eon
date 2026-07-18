// Package searxng adapts the provider-neutral search port to SearXNG's JSON
// search endpoint. Returned titles and snippets remain untrusted source data.
package searxng

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"motor-autonomo/internal/port"
)

const (
	defaultMaxResponseBytes int64 = 1 << 20
	defaultClientTimeout          = 30 * time.Second
)

type Config struct {
	BaseURL          string
	Client           *http.Client
	MaxResponseBytes int64
}

type Searcher struct {
	endpoint         *url.URL
	client           *http.Client
	maxResponseBytes int64
}

type ErrorKind string

const (
	ErrorInvalidRequest   ErrorKind = "INVALID_REQUEST"
	ErrorTransport        ErrorKind = "TRANSPORT"
	ErrorHTTP             ErrorKind = "HTTP"
	ErrorResponseTooLarge ErrorKind = "RESPONSE_TOO_LARGE"
	ErrorInvalidResponse  ErrorKind = "INVALID_RESPONSE"
)

type Error struct {
	Kind       ErrorKind
	StatusCode int
	Retryable  bool
}

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("searxng search: %s (status %d)", e.Kind, e.StatusCode)
	}
	return "searxng search: " + string(e.Kind)
}

func New(config Config) (*Searcher, error) {
	endpoint, err := url.Parse(config.BaseURL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return nil, errors.New("base URL must be an absolute HTTP(S) URL")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/search"
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	limit := config.MaxResponseBytes
	if limit == 0 {
		limit = defaultMaxResponseBytes
	}
	if limit < 1 {
		return nil, errors.New("max response bytes must be positive")
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: defaultClientTimeout}
	}
	return &Searcher{endpoint: endpoint, client: client, maxResponseBytes: limit}, nil
}

func (s *Searcher) Search(ctx context.Context, request port.SearchRequest) (port.SearchResult, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" || request.Limit < 1 {
		return port.SearchResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	target := *s.endpoint
	values := target.Query()
	values.Set("q", query)
	values.Set("format", "json")
	target.RawQuery = values.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return port.SearchResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	httpRequest.Header.Set("Accept", "application/json")
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return port.SearchResult{}, &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, s.maxResponseBytes+1))
		return port.SearchResult{}, &Error{Kind: ErrorHTTP, StatusCode: response.StatusCode, Retryable: response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, s.maxResponseBytes+1))
	if err != nil {
		return port.SearchResult{}, &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(body)) > s.maxResponseBytes {
		return port.SearchResult{}, &Error{Kind: ErrorResponseTooLarge}
	}
	var decoded struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	if err := decoder.Decode(&decoded); err != nil {
		return port.SearchResult{}, &Error{Kind: ErrorInvalidResponse}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return port.SearchResult{}, &Error{Kind: ErrorInvalidResponse}
	}
	result := port.SearchResult{Provider: "searxng"}
	for _, item := range decoded.Results {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.URL) == "" {
			return port.SearchResult{}, &Error{Kind: ErrorInvalidResponse}
		}
		result.Hits = append(result.Hits, port.SearchHit{Title: item.Title, URL: item.URL, Snippet: item.Content})
		if len(result.Hits) == request.Limit {
			break
		}
	}
	return result, nil
}
