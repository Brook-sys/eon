// Package replay provides deterministic web search fixtures for offline tests
// and reproducible evaluation runs.
package replay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"motor-autonomo/internal/port"
)

type Fixture struct {
	Query  string
	Result port.SearchResult
}

type Searcher struct {
	mu       sync.Mutex
	fixtures map[string]port.SearchResult
	requests []port.SearchRequest
}

func New(fixtures ...Fixture) (*Searcher, error) {
	s := &Searcher{fixtures: make(map[string]port.SearchResult, len(fixtures))}
	for _, fixture := range fixtures {
		query := strings.TrimSpace(fixture.Query)
		if query == "" || fixture.Result.Provider == "" || fixture.Result.FixtureKey == "" {
			return nil, errors.New("replay fixture requires query, provider and fixture key")
		}
		if _, exists := s.fixtures[query]; exists {
			return nil, fmt.Errorf("duplicate replay query %q", query)
		}
		result, err := cloneAndValidate(fixture.Result)
		if err != nil {
			return nil, fmt.Errorf("fixture %q: %w", query, err)
		}
		s.fixtures[query] = result
	}
	return s, nil
}

func (s *Searcher) Search(ctx context.Context, request port.SearchRequest) (port.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return port.SearchResult{}, err
	}
	query := strings.TrimSpace(request.Query)
	if query == "" || request.Limit < 1 {
		return port.SearchResult{}, errors.New("search requires a query and positive limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	fixture, ok := s.fixtures[query]
	if !ok {
		return port.SearchResult{}, fmt.Errorf("replay fixture not found for query %q", query)
	}
	result, _ := cloneAndValidate(fixture)
	if len(result.Hits) > request.Limit {
		result.Hits = result.Hits[:request.Limit]
	}
	return result, nil
}

func (s *Searcher) Requests() []port.SearchRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]port.SearchRequest(nil), s.requests...)
}

func cloneAndValidate(result port.SearchResult) (port.SearchResult, error) {
	cloned := result
	cloned.Hits = append([]port.SearchHit(nil), result.Hits...)
	for index, hit := range cloned.Hits {
		if strings.TrimSpace(hit.Title) == "" || strings.TrimSpace(hit.URL) == "" {
			return port.SearchResult{}, fmt.Errorf("hit %d requires title and URL", index)
		}
	}
	return cloned, nil
}
