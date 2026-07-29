package secretvault_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/secretvault"
)

func TestVault_TTLAndExpiration(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "ttl_vault.json")

	now := time.Now()
	clock := func() time.Time { return now }

	v, err := secretvault.NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock failed: %v", err)
	}

	password := "master-password-12345"
	if err := v.Initialize(password); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	secretName := "temp_api_key"
	secretVal := "secret-token-val"

	// Put secret with 5-minute TTL
	ttl := 5 * time.Minute
	if err := v.PutWithTTL(secretName, secretVal, ttl); err != nil {
		t.Fatalf("PutWithTTL failed: %v", err)
	}

	// Immediate resolve before expiration
	val, err := v.Resolve(secretName)
	if err != nil {
		t.Fatalf("Resolve failed before expiration: %v", err)
	}
	if val != secretVal {
		t.Fatalf("Resolve expected %q, got %q", secretVal, val)
	}

	// Status before expiration
	st := v.Status()
	if len(st.Secrets) != 1 {
		t.Fatalf("Status expected 1 secret, got %d", len(st.Secrets))
	}
	if st.Secrets[0].Expired {
		t.Fatalf("Status secret should not be marked expired yet")
	}

	// Advance clock past TTL
	now = now.Add(6 * time.Minute)

	// Resolve after expiration
	_, err = v.Resolve(secretName)
	if err == nil || err != secretvault.ErrSecretExpired {
		t.Fatalf("Resolve expected ErrSecretExpired, got: %v", err)
	}

	// Status after expiration
	st = v.Status()
	if len(st.Secrets) != 1 {
		t.Fatalf("Status expected 1 secret, got %d", len(st.Secrets))
	}
	if !st.Secrets[0].Expired {
		t.Fatalf("Status expected Expired=true after TTL pass")
	}

	// Invalid TTL <= 0
	if err := v.PutWithTTL("bad_ttl", "val", -1*time.Minute); err != secretvault.ErrInvalidTTL {
		t.Fatalf("PutWithTTL expected ErrInvalidTTL, got: %v", err)
	}
}

func TestVault_AuditLogAndHTTP(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "audit_vault.json")

	now := time.Now()
	clock := func() time.Time { return now }

	v, err := secretvault.NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock failed: %v", err)
	}

	password := "master-password-12345"
	if err := v.Initialize(password); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := v.Put("token1", "secret-val-1"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if _, err := v.Resolve("token1"); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if err := v.Delete("token1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	audit := v.AuditLog()
	if len(audit) < 4 {
		t.Fatalf("AuditLog expected at least 4 events, got %d", len(audit))
	}

	// Verify events don't leak secret value
	for _, evt := range audit {
		if evt.Action == "" || evt.Status == "" {
			t.Fatalf("AuditEvent fields missing: %+v", evt)
		}
		raw, _ := json.Marshal(evt)
		if bytes.Contains(raw, []byte("secret-val-1")) || bytes.Contains(raw, []byte(password)) {
			t.Fatalf("AuditEvent leaked sensitive data! %s", string(raw))
		}
	}

	// HTTP API integration testing
	srv := httptest.NewServer(secretvault.HTTP{Vault: v}.Handler())
	defer srv.Close()

	// GET /audit
	res, err := http.Get(srv.URL + "/audit")
	if err != nil {
		t.Fatalf("GET /audit failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /audit status expected 200, got %d", res.StatusCode)
	}

	var events []secretvault.AuditEvent
	if err := json.NewDecoder(res.Body).Decode(&events); err != nil {
		t.Fatalf("Decode audit events failed: %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("HTTP GET /audit expected >=4 events, got %d", len(events))
	}

	// PUT /secrets/temp with TTL JSON
	putBody, _ := json.Marshal(map[string]string{"value": "http-secret-val", "ttl": "100ms"})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/secrets/temp", bytes.NewReader(putBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /secrets/temp with TTL failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT /secrets/temp expected 204, got %d", resp.StatusCode)
	}

	// Verify status includes expires_at
	st := v.Status()
	found := false
	for _, m := range st.Secrets {
		if m.Name == "temp" {
			found = true
			if m.ExpiresAt.IsZero() {
				t.Fatalf("Expected non-zero ExpiresAt for temp secret")
			}
		}
	}
	if !found {
		t.Fatalf("Secret 'temp' not found in status")
	}

	// Advance clock past 100ms
	now = now.Add(200 * time.Millisecond)

	// Resolve expired secret
	_, err = v.Resolve("temp")
	if err != secretvault.ErrSecretExpired {
		t.Fatalf("Expected ErrSecretExpired after advance, got %v", err)
	}

	// Test POST /purge-expired via HTTP API
	reqPurge, _ := http.NewRequest(http.MethodPost, srv.URL+"/purge-expired", nil)
	respPurge, err := http.DefaultClient.Do(reqPurge)
	if err != nil {
		t.Fatalf("POST /purge-expired failed: %v", err)
	}
	defer respPurge.Body.Close()
	if respPurge.StatusCode != http.StatusOK {
		t.Fatalf("POST /purge-expired expected 200, got %d", respPurge.StatusCode)
	}

	var purgeRes map[string]int
	if err := json.NewDecoder(respPurge.Body).Decode(&purgeRes); err != nil {
		t.Fatalf("Decode /purge-expired response failed: %v", err)
	}
	if purgeRes["purged"] < 1 {
		t.Fatalf("Expected purged >= 1, got %d", purgeRes["purged"])
	}
}

func TestVault_PurgeAndRotateWithTTL(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "purge_rotate_vault.json")

	now := time.Now()
	clock := func() time.Time { return now }

	v, err := secretvault.NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock failed: %v", err)
	}

	password := "master-password-12345"
	if err := v.Initialize(password); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := v.Put("key1", "val1"); err != nil {
		t.Fatalf("Put key1 failed: %v", err)
	}
	if err := v.PutWithTTL("key2", "val2", 1*time.Minute); err != nil {
		t.Fatalf("PutWithTTL key2 failed: %v", err)
	}

	// Rotate key1 with TTL
	if err := v.RotateWithTTL("key1", "val1-rotated", 2*time.Minute); err != nil {
		t.Fatalf("RotateWithTTL key1 failed: %v", err)
	}

	val, err := v.Resolve("key1")
	if err != nil || val != "val1-rotated" {
		t.Fatalf("Resolve key1 expected 'val1-rotated', got val=%q err=%v", val, err)
	}

	// Rotate with invalid TTL
	if err := v.RotateWithTTL("key1", "val1-bad", -1*time.Second); err != secretvault.ErrInvalidTTL {
		t.Fatalf("RotateWithTTL expected ErrInvalidTTL, got %v", err)
	}

	// Advance clock by 90 seconds (key2 expired, key1 still valid)
	now = now.Add(90 * time.Second)

	purged, err := v.PurgeExpired()
	if err != nil {
		t.Fatalf("PurgeExpired failed: %v", err)
	}
	if purged != 1 {
		t.Fatalf("PurgeExpired expected 1, got %d", purged)
	}

	// Verify key2 is completely deleted from vault
	st := v.Status()
	for _, m := range st.Secrets {
		if m.Name == "key2" {
			t.Fatalf("key2 should have been purged")
		}
	}

	// Advance clock by 60 more seconds (key1 now expired)
	now = now.Add(60 * time.Second)
	purged, err = v.PurgeExpired()
	if err != nil {
		t.Fatalf("PurgeExpired second run failed: %v", err)
	}
	if purged != 1 {
		t.Fatalf("PurgeExpired expected 1 for key1, got %d", purged)
	}
}

func TestVault_BatchResolve(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "batch_vault.json")

	now := time.Now()
	clock := func() time.Time { return now }

	v, err := secretvault.NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock failed: %v", err)
	}

	password := "master-password-12345"
	if err := v.Initialize(password); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Add 3 secrets: normal, to expire, and omit a 4th
	if err := v.Put("k1", "val1"); err != nil {
		t.Fatalf("Put k1 failed: %v", err)
	}
	if err := v.PutWithTTL("k2", "val2", 1*time.Minute); err != nil {
		t.Fatalf("Put k2 failed: %v", err)
	}
	if err := v.Put("k3", "val3"); err != nil {
		t.Fatalf("Put k3 failed: %v", err)
	}

	// Resolve batch when unlocked before expiration
	res := v.ResolveAll([]string{"k1", "k2", "missing"})
	if len(res) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(res))
	}
	if res[0].Name != "k1" || res[0].Value != "val1" || res[0].Error != "" {
		t.Fatalf("Unexpected res[0]: %+v", res[0])
	}
	if res[1].Name != "k2" || res[1].Value != "val2" || res[1].Error != "" {
		t.Fatalf("Unexpected res[1]: %+v", res[1])
	}
	if res[2].Name != "missing" || res[2].Value != "" || res[2].Error == "" {
		t.Fatalf("Unexpected res[2]: %+v", res[2])
	}

	// Advance clock past TTL for k2
	now = now.Add(2 * time.Minute)

	res = v.ResolveAll([]string{"k1", "k2", "k3"})
	if res[0].Error != "" || res[0].Value != "val1" {
		t.Fatalf("res[0] expected success, got %+v", res[0])
	}
	if res[1].Error != secretvault.ErrSecretExpired.Error() {
		t.Fatalf("res[1] expected expired, got %+v", res[1])
	}
	if res[2].Error != "" || res[2].Value != "val3" {
		t.Fatalf("res[2] expected success, got %+v", res[2])
	}

	// Resolve batch when locked
	v.Lock()
	res = v.ResolveAll([]string{"k1", "k3"})
	if res[0].Error != secretvault.ErrLocked.Error() || res[1].Error != secretvault.ErrLocked.Error() {
		t.Fatalf("Expected locked error for all items, got %+v", res)
	}

	// HTTP API testing POST /resolve
	if err := v.Unlock(password); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}
	srv := httptest.NewServer(secretvault.HTTP{Vault: v}.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string][]string{"names": {"k1", "missing"}})
	resp, err := http.Post(srv.URL+"/resolve", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /resolve failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /resolve expected 200, got %d", resp.StatusCode)
	}

	var httpRes []secretvault.ResolveResult
	if err := json.NewDecoder(resp.Body).Decode(&httpRes); err != nil {
		t.Fatalf("Decode /resolve response failed: %v", err)
	}
	if len(httpRes) != 2 || httpRes[0].Value != "val1" || httpRes[1].Error == "" {
		t.Fatalf("Unexpected HTTP resolve results: %+v", httpRes)
	}
}
