package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVault_BatchSearch(t *testing.T) {
	v := newUnlockedVault(t)

	// Locked vault returns ErrLocked
	v.Lock()
	if _, err := v.BatchSearch([]BatchSearchItem{{Prefix: "prod/"}}); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	// Unlock
	if err := v.Unlock("super-secret-password"); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	_ = v.Put("prod/db/pass", "val1")
	_ = v.Put("prod/api/token", "val2")
	_ = v.Put("staging/db/pass", "val3")

	items := []BatchSearchItem{
		{Prefix: "prod/"},
		{Substring: "token"},
		{Prefix: "dev/"},
	}

	results, err := v.BatchSearch(items)
	if err != nil {
		t.Fatalf("BatchSearch failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if len(results[0].Secrets) != 2 || results[0].Secrets[0].Name != "prod/api/token" {
		t.Fatalf("prod/ unexpected result: %+v", results[0])
	}

	if len(results[1].Secrets) != 1 || results[1].Secrets[0].Name != "prod/api/token" {
		t.Fatalf("token unexpected result: %+v", results[1])
	}

	if len(results[2].Secrets) != 0 {
		t.Fatalf("dev/ unexpected result: %+v", results[2])
	}
}

func TestHTTP_BatchSearch(t *testing.T) {
	v := newUnlockedVault(t)
	_ = v.Put("prod/db/pass", "v1")

	h := HTTP{Vault: v}
	ts := httptest.NewServer(h.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"items": []BatchSearchItem{
			{Prefix: "prod/"},
		},
	})

	resp, err := http.Post(ts.URL+"/secrets/batch-search", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var results []BatchSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(results) != 1 || len(results[0].Secrets) != 1 {
		t.Fatalf("unexpected HTTP result: %+v", results)
	}
}
