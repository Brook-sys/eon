package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVault_BatchAuditSummary(t *testing.T) {
	v := newUnlockedVault(t)

	// Add audit events
	_ = v.Put("k1", "v1")
	_ = v.Put("k2", "v2")
	_ = v.Delete("k1")

	items := []BatchAuditSummaryItem{
		{SecretName: "k1"},
		{Action: "put"},
		{SecretName: "invalid/../name"},
	}

	results := v.BatchAuditSummary(items)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Item 0: k1 (put + delete = 2 matched events)
	if results[0].Summary.MatchedEvents < 2 || results[0].Error != "" {
		t.Fatalf("item 0 unexpected: %+v", results[0])
	}

	// Item 1: Action put (put k1 + put k2 = 2 matched events)
	if results[1].Summary.MatchedEvents != 2 || results[1].Error != "" {
		t.Fatalf("item 1 unexpected: %+v", results[1])
	}

	// Item 2: invalid name (Error non-empty)
	if results[2].Error == "" {
		t.Fatalf("item 2 expected error for invalid name")
	}

	// Works even when locked (inspects in-memory audit log)
	v.Lock()
	lockedResults := v.BatchAuditSummary([]BatchAuditSummaryItem{{SecretName: "k1"}})
	if len(lockedResults) != 1 || lockedResults[0].Summary.MatchedEvents == 0 {
		t.Fatalf("locked vault expected matched events in BatchAuditSummary: %+v", lockedResults)
	}
}

func TestHTTP_BatchAuditSummary(t *testing.T) {
	v := newUnlockedVault(t)
	_ = v.Put("k1", "v1")

	h := HTTP{Vault: v}
	ts := httptest.NewServer(h.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"items": []BatchAuditSummaryItem{
			{SecretName: "k1"},
		},
	})

	resp, err := http.Post(ts.URL+"/audit/batch-summary", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var results []BatchAuditSummaryResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(results) != 1 || results[0].Summary.MatchedEvents == 0 {
		t.Fatalf("unexpected HTTP result: %+v", results)
	}
}
