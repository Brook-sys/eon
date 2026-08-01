package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVault_BatchRotate(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Unlocked error test
	_, err = v.BatchRotate([]BatchRotateItem{{Name: "k1", Value: "v1"}})
	if err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Seed secrets k1 and k2
	if err := v.Put("k1", "val1"); err != nil {
		t.Fatalf("Put k1 failed: %v", err)
	}
	if err := v.Put("k2", "val2"); err != nil {
		t.Fatalf("Put k2 failed: %v", err)
	}

	// Batch rotate k1 and k2, plus missing k3
	res, err := v.BatchRotate([]BatchRotateItem{
		{Name: "k1", Value: "new-val1", TTL: 1 * time.Hour},
		{Name: "k2", Value: "new-val2"},
		{Name: "k3", Value: "val3"},
	})
	if err != nil {
		t.Fatalf("BatchRotate failed: %v", err)
	}

	if len(res.Rotated) != 2 || res.Rotated[0] != "k1" || res.Rotated[1] != "k2" {
		t.Fatalf("unexpected Rotated: %v", res.Rotated)
	}
	if len(res.Errors) != 1 || res.Errors[0] != "k3: not found" {
		t.Fatalf("unexpected Errors: %v", res.Errors)
	}

	// Verify values updated
	val1, err := v.Resolve("k1")
	if err != nil || val1 != "new-val1" {
		t.Fatalf("k1 resolve failed, got %q, err %v", val1, err)
	}
	val2, err := v.Resolve("k2")
	if err != nil || val2 != "new-val2" {
		t.Fatalf("k2 resolve failed, got %q, err %v", val2, err)
	}

	// Invalid items validation
	resInv, err := v.BatchRotate([]BatchRotateItem{
		{Name: "a/../b", Value: "v"},
		{Name: "k1", Value: ""},
		{Name: "k2", Value: "v", TTL: -1 * time.Second},
	})
	if err != nil {
		t.Fatalf("BatchRotate failed: %v", err)
	}
	if len(resInv.Rotated) != 0 || len(resInv.Errors) != 3 {
		t.Fatalf("unexpected resInv: %v", resInv)
	}
}

func TestHTTP_BatchRotate(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("k1", "val1"); err != nil {
		t.Fatalf("Put k1 failed: %v", err)
	}

	handler := HTTP{Vault: v}.Handler()

	// 200 OK batch rotate
	body, _ := json.Marshal(batchRotateRequest{
		Items: []BatchRotateItem{
			{Name: "k1", Value: "rotated-val1"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/secrets/batch-rotate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var res BatchRotateResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(res.Rotated) != 1 || res.Rotated[0] != "k1" {
		t.Fatalf("unexpected Rotated: %v", res.Rotated)
	}

	// 400 Bad Request (empty items)
	reqEmpty := httptest.NewRequest(http.MethodPost, "/secrets/batch-rotate", bytes.NewReader([]byte(`{"items":[]}`)))
	reqEmpty.Header.Set("Content-Type", "application/json")
	reqEmpty.RemoteAddr = "127.0.0.1:12345"
	recEmpty := httptest.NewRecorder()
	handler.ServeHTTP(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d", recEmpty.Code)
	}

	// 423 Locked
	v.Lock()
	reqLocked := httptest.NewRequest(http.MethodPost, "/secrets/batch-rotate", bytes.NewReader(body))
	reqLocked.Header.Set("Content-Type", "application/json")
	reqLocked.RemoteAddr = "127.0.0.1:12345"
	recLocked := httptest.NewRecorder()
	handler.ServeHTTP(recLocked, reqLocked)
	if recLocked.Code != http.StatusLocked {
		t.Fatalf("expected HTTP 423, got %d", recLocked.Code)
	}
}

func TestVault_BatchMetadata(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Locked error
	_, err = v.BatchMetadata([]string{"k1"})
	if err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := v.PutWithTTL("k1", "val1", 100*time.Millisecond); err != nil {
		t.Fatalf("PutWithTTL k1 failed: %v", err)
	}
	if err := v.Put("k2", "val2"); err != nil {
		t.Fatalf("Put k2 failed: %v", err)
	}

	// Batch metadata
	results, err := v.BatchMetadata([]string{"k1", "k2", "k3", "invalid/../name"})
	if err != nil {
		t.Fatalf("BatchMetadata failed: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Check k1
	if results[0].Name != "k1" || results[0].Error != "" || results[0].ExpiresAt.IsZero() {
		t.Fatalf("unexpected results[0]: %+v", results[0])
	}
	// Check k2
	if results[1].Name != "k2" || results[1].Error != "" || !results[1].ExpiresAt.IsZero() {
		t.Fatalf("unexpected results[1]: %+v", results[1])
	}
	// Check k3 (missing)
	if results[2].Name != "k3" || results[2].Error != os.ErrNotExist.Error() {
		t.Fatalf("unexpected results[2]: %+v", results[2])
	}
	// Check invalid name
	if results[3].Name != "invalid/../name" || results[3].Error == "" {
		t.Fatalf("unexpected results[3]: %+v", results[3])
	}

	// Wait for k1 to expire and check Expired boolean
	time.Sleep(120 * time.Millisecond)
	resultsExp, err := v.BatchMetadata([]string{"k1"})
	if err != nil {
		t.Fatalf("BatchMetadata post-expire failed: %v", err)
	}
	if !resultsExp[0].Expired {
		t.Fatalf("expected k1 to be marked Expired=true, got %+v", resultsExp[0])
	}
}

func TestHTTP_BatchMetadata(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("k1", "val1"); err != nil {
		t.Fatalf("Put k1 failed: %v", err)
	}

	handler := HTTP{Vault: v}.Handler()

	body, _ := json.Marshal(batchMetadataRequest{Names: []string{"k1", "k2"}})
	req := httptest.NewRequest(http.MethodPost, "/secrets/batch-metadata", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Metadata []BatchMetadataResult `json:"metadata"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(resp.Metadata) != 2 || resp.Metadata[0].Name != "k1" || resp.Metadata[1].Name != "k2" {
		t.Fatalf("unexpected metadata: %+v", resp.Metadata)
	}
}

func TestVault_AuditFilterUntilAndLimit(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	t0 := time.Now().UTC()
	v.Put("k1", "v1")
	v.Put("k2", "v2")
	v.Put("k3", "v3")

	allEvts := v.AuditLogFiltered(AuditFilter{})
	if len(allEvts) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(allEvts))
	}

	// Filter with Limit=2
	limEvts := v.AuditLogFiltered(AuditFilter{Limit: 2})
	if len(limEvts) != 2 {
		t.Fatalf("expected 2 events with limit=2, got %d", len(limEvts))
	}

	// Filter with Until
	untilEvts := v.AuditLogFiltered(AuditFilter{Until: t0.Add(-1 * time.Second)})
	if len(untilEvts) != 0 {
		t.Fatalf("expected 0 events before t0, got %d", len(untilEvts))
	}

	// HTTP GET /audit?limit=2
	handler := HTTP{Vault: v}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/audit?limit=2", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}
	var httpEvts []AuditEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &httpEvts); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(httpEvts) != 2 {
		t.Fatalf("expected 2 events from HTTP limit=2, got %d", len(httpEvts))
	}
}
