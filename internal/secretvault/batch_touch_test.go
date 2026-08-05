package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVault_BatchTouch(t *testing.T) {
	v := newUnlockedVault(t)

	// Locked vault returns ErrLocked
	v.Lock()
	if _, err := v.BatchTouch([]BatchTouchItem{{Name: "k1", TTL: time.Hour}}); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	// Unlock
	if err := v.Unlock("super-secret-password"); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	// Setup secrets
	_ = v.Put("k1", "val1")
	_ = v.Put("k2", "val2")

	items := []BatchTouchItem{
		{Name: "k1", TTL: 2 * time.Hour},
		{Name: "k2", TTL: 30 * time.Minute},
		{Name: "missing", TTL: time.Hour},
		{Name: "invalid/../name", TTL: time.Hour},
		{Name: "k1", TTL: -1 * time.Second},
	}

	resp, err := v.BatchTouch(items)
	if err != nil {
		t.Fatalf("BatchTouch failed: %v", err)
	}

	if resp.Processed != 5 {
		t.Fatalf("expected 5 processed, got %d", resp.Processed)
	}
	if resp.Touched != 2 {
		t.Fatalf("expected 2 touched, got %d", resp.Touched)
	}
	if resp.Failed != 3 {
		t.Fatalf("expected 3 failed, got %d", resp.Failed)
	}

	// Verify k1 expiration updated
	meta1, err := v.SecretMetadata("k1")
	if err != nil {
		t.Fatalf("SecretMetadata k1 failed: %v", err)
	}
	if meta1.ExpiresAt.IsZero() {
		t.Fatalf("expected k1 to have non-zero ExpiresAt")
	}

	// Verify audit log recorded touch
	history, err := v.SecretHistory("k1")
	if err != nil {
		t.Fatalf("SecretHistory failed: %v", err)
	}
	hasTouch := false
	for _, evt := range history {
		if evt.Action == "touch" && evt.Status == "success" {
			hasTouch = true
			break
		}
	}
	if !hasTouch {
		t.Fatalf("expected touch event in audit history for k1")
	}
}

func TestHTTP_BatchTouch(t *testing.T) {
	v := newUnlockedVault(t)
	_ = v.Put("k1", "val1")

	h := HTTP{Vault: v}
	ts := httptest.NewServer(h.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"name": "k1", "ttl": 3600000000000}, // 1 hour in ns
		},
	})

	resp, err := http.Post(ts.URL+"/secrets/batch-touch", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var batchResp BatchTouchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if batchResp.Touched != 1 || len(batchResp.Results) != 1 {
		t.Fatalf("unexpected HTTP result: %+v", batchResp)
	}
}
