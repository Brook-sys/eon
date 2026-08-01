package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestVault_BatchExists(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Locked vault must return ErrLocked.
	if _, err := v.BatchExists([]string{"k1"}); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := v.Put("k1", "val1"); err != nil {
		t.Fatalf("Put k1 failed: %v", err)
	}
	if err := v.Put("k2", "val2"); err != nil {
		t.Fatalf("Put k2 failed: %v", err)
	}

	res, err := v.BatchExists([]string{"k1", "missing", "k2", "bad//name"})
	if err != nil {
		t.Fatalf("BatchExists failed: %v", err)
	}
	if len(res) != 4 {
		t.Fatalf("expected 4 results, got %d", len(res))
	}
	if !res[0].Exists || res[0].Error != "" {
		t.Fatalf("k1 should exist without error: %+v", res[0])
	}
	if res[1].Exists || res[1].Error != "" {
		t.Fatalf("missing should not exist without error: %+v", res[1])
	}
	if !res[2].Exists || res[2].Error != "" {
		t.Fatalf("k2 should exist without error: %+v", res[2])
	}
	if res[3].Error == "" {
		t.Fatalf("invalid name should produce error: %+v", res[3])
	}

	// BatchExists must not expose values: marshal and check payload.
	buf, _ := json.Marshal(res)
	var decoded []map[string]any
	if err := json.Unmarshal(buf, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, m := range decoded {
		if _, leaked := m["value"]; leaked {
			t.Fatalf("value leaked in ExistsResult: %v", m)
		}
	}
}

func TestHTTP_BatchExistsEndpoint(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("api-key", "sk-test"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	h := HTTP{Vault: v}.Handler()

	newReq := func(body []byte) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/secrets/batch-exists", bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		return req
	}

	// Empty batch rejected.
	req := newReq([]byte(`{"names":[]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty names, got %d", rec.Code)
	}

	// Oversized batch rejected.
	names := make([]string, 101)
	for i := range names {
		names[i] = "n"
	}
	body, _ := json.Marshal(map[string]any{"names": names})
	req = newReq(body)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 101 names, got %d", rec.Code)
	}

	// Happy path.
	req = newReq([]byte(`{"names":["api-key","nope"]}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Results []ExistsResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(out.Results) != 2 || !out.Results[0].Exists || out.Results[1].Exists {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
}
