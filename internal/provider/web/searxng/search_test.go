package searxng_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/web/searxng"
)

func TestSearcherUsesJSONEndpointAndBoundsHits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "bounded research" || r.URL.Query().Get("format") != "json" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"A","url":"https://example.test/a","content":"snippet A"},{"title":"B","url":"https://example.test/b","content":"snippet B"}]}`))
	}))
	defer server.Close()
	searcher, err := searxng.New(searxng.Config{BaseURL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := searcher.Search(context.Background(), port.SearchRequest{Query: "bounded research", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "searxng" || len(result.Hits) != 1 || result.Hits[0].Snippet != "snippet A" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestSearcherClassifiesBoundedFailuresWithoutLeakingBody(t *testing.T) {
	tests := []struct {
		name, body string
		status     int
		limit      int64
		kind       searxng.ErrorKind
	}{
		{name: "status", body: "secret diagnostic", status: http.StatusTooManyRequests, kind: searxng.ErrorHTTP},
		{name: "oversize", body: strings.Repeat("x", 9), limit: 8, kind: searxng.ErrorResponseTooLarge},
		{name: "invalid", body: `{`, kind: searxng.ErrorInvalidResponse},
		{name: "trailing", body: `{"results":[]} {}`, kind: searxng.ErrorInvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			searcher, _ := searxng.New(searxng.Config{BaseURL: server.URL, Client: server.Client(), MaxResponseBytes: test.limit})
			_, err := searcher.Search(context.Background(), port.SearchRequest{Query: "q", Limit: 1})
			var searchError *searxng.Error
			if !errors.As(err, &searchError) || searchError.Kind != test.kind {
				t.Fatalf("error = %#v, want %s", err, test.kind)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked body: %v", err)
			}
		})
	}
}
