package replay_test

import (
	"context"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/web/replay"
)

func TestSearcherReplaysExactBoundedFixtureWithoutAliasing(t *testing.T) {
	fixture := replay.Fixture{Query: "primary sources", Result: port.SearchResult{Provider: "fixture", FixtureKey: "search-v1", Hits: []port.SearchHit{{Title: "A", URL: "https://example.test/a", Snippet: "untrusted text"}, {Title: "B", URL: "https://example.test/b"}}}}
	searcher, err := replay.New(fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := searcher.Search(context.Background(), port.SearchRequest{Query: "primary sources", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Title != "A" || result.FixtureKey != "search-v1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	result.Hits[0].Title = "mutated"
	again, err := searcher.Search(context.Background(), port.SearchRequest{Query: "primary sources", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Hits) != 2 || again.Hits[0].Title != "A" {
		t.Fatalf("fixture was aliased: %+v", again)
	}
	if len(searcher.Requests()) != 2 {
		t.Fatalf("requests not captured: %+v", searcher.Requests())
	}
}

func TestSearcherRejectsUnknownOrInvalidFixtures(t *testing.T) {
	if _, err := replay.New(replay.Fixture{Query: "x", Result: port.SearchResult{Provider: "fixture", FixtureKey: "v1", Hits: []port.SearchHit{{URL: "https://example.test"}}}}); err == nil {
		t.Fatal("expected invalid fixture")
	}
	searcher, err := replay.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := searcher.Search(context.Background(), port.SearchRequest{Query: "missing", Limit: 1}); err == nil {
		t.Fatal("expected missing fixture")
	}
}
