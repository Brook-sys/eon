package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVault_BatchAuditFilter(t *testing.T) {
	v := newUnlockedVault(t)

	// Locked vault returns ErrLocked
	v.Lock()
	if _, err := v.BatchAuditFilter([]BatchAuditFilterItem{{SecretName: "k1"}}); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	// Unlock
	if err := v.Unlock("super-secret-password"); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	// Perform secret operations to generate audit log events with SecretName
	_ = v.Put("k1", "val1")
	_ = v.Put("k2", "val2")
	_ = v.Delete("k1")

	// Query BatchAuditFilter
	items := []BatchAuditFilterItem{
		{SecretName: "k1"},
		{SecretName: "k2", Action: "put"},
		{SecretName: "missing"},
		{SecretName: "invalid/../name"},
	}

	results, err := v.BatchAuditFilter(items)
	if err != nil {
		t.Fatalf("BatchAuditFilter failed: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Check k1 (should have put + delete events)
	if !results[0].Found || len(results[0].Events) < 2 {
		t.Fatalf("k1 unexpected result: %+v", results[0])
	}

	// Check k2 (should have 1 put event)
	if !results[1].Found || len(results[1].Events) != 1 {
		t.Fatalf("k2 unexpected result: %+v", results[1])
	}

	// Check missing (Found false, empty events)
	if results[2].Found || len(results[2].Events) != 0 {
		t.Fatalf("missing unexpected result: %+v", results[2])
	}

	// Check invalid name (Error non-empty)
	if results[3].Error == "" {
		t.Fatalf("invalid name expected Error non-empty")
	}
}

func TestHTTP_BatchAuditFilter(t *testing.T) {
	v := newUnlockedVault(t)

	_ = v.Put("k1", "v1")

	h := HTTP{Vault: v}
	ts := httptest.NewServer(h.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"items": []BatchAuditFilterItem{
			{SecretName: "k1"},
		},
	})

	resp, err := http.Post(ts.URL+"/audit/batch-filter", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var results []BatchAuditFilterResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(results) != 1 || !results[0].Found {
		t.Fatalf("unexpected HTTP result: %+v", results)
	}
}
