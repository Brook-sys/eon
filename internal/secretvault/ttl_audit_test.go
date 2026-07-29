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
}
