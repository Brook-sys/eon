package port

import "context"

// SearchRequest and SearchResult keep web discovery provider-neutral. Search
// snippets are untrusted source data, never instructions or executable actions.
type SearchRequest struct {
	Query string
	Limit int
}

type SearchHit struct {
	Title   string
	URL     string
	Snippet string
}

type SearchResult struct {
	Hits       []SearchHit
	Provider   string
	FixtureKey string
}

type WebSearcher interface {
	Search(context.Context, SearchRequest) (SearchResult, error)
}

// FetchRequest and FetchResult are the bounded acquisition contract. The
// adapter must return the final URL after validated redirects and exact bytes.
type FetchRequest struct {
	URL string
}

type FetchResult struct {
	FinalURL     string
	MediaType    string
	ETag         string
	LastModified string
	Content      []byte
}

type WebFetcher interface {
	Fetch(context.Context, FetchRequest) (FetchResult, error)
}
