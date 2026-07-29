package secretvault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVaultEncryptedRoundTripAndMetadataRedaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.vault")
	v, _ := New(path)
	if err := v.Initialize("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("provider/groq/api-key", "super-secret-token"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "super-secret-token") {
		t.Fatal("vault persisted plaintext")
	}
	st := v.Status()
	if len(st.Secrets) != 1 || st.Secrets[0].Name != "provider/groq/api-key" {
		t.Fatalf("status=%+v", st)
	}
	encoded, _ := json.Marshal(st)
	if strings.Contains(string(encoded), "super-secret-token") {
		t.Fatal("metadata leaked secret")
	}
	v.Lock()
	if _, err := v.Resolve("provider/groq/api-key"); err != ErrLocked {
		t.Fatalf("locked resolve=%v", err)
	}
	if err := v.Unlock("wrong password value"); err != ErrInvalidPassword {
		t.Fatalf("wrong password=%v", err)
	}
	if err := v.Unlock("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	got, err := v.Resolve("provider/groq/api-key")
	if err != nil || got != "super-secret-token" {
		t.Fatalf("resolve=%q %v", got, err)
	}
}

func TestInactivityClockLocksExactlyAtBoundaryAndActivityExtendsIt(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	v, err := NewWithClock(filepath.Join(t.TempDir(), "vault"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err = v.Initialize("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err = v.Put("a", "secret"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(autoLockAfter - time.Nanosecond)
	if _, err = v.Resolve("a"); err != nil {
		t.Fatalf("before boundary: %v", err)
	}
	// Resolve is activity and starts a fresh inactivity interval.
	now = now.Add(autoLockAfter - time.Nanosecond)
	if v.Status().Locked {
		t.Fatal("locked before extended boundary")
	}
	now = now.Add(time.Nanosecond)
	if !v.Status().Locked {
		t.Fatal("vault did not lock exactly at inactivity boundary")
	}
}

func TestConcurrentHandlesMergeWritesWithoutLostUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault")
	first, _ := New(path)
	if err := first.Initialize("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	second, _ := New(path)
	if err := second.Unlock("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for _, item := range []struct {
		v    *Vault
		name string
	}{{first, "one"}, {second, "two"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- item.v.Put(item.name, "value-"+item.name)
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	reopened, _ := New(path)
	if err := reopened.Unlock("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if got, err := reopened.Resolve(name); err != nil || got != "value-"+name {
			t.Fatalf("%s=%q, %v", name, got, err)
		}
	}
}

func TestInterruptedTemporaryWriteDoesNotDamageCommittedVault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault")
	v, _ := New(path)
	if err := v.Initialize("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("provider/key", "committed-secret"); err != nil {
		t.Fatal(err)
	}
	// A process killed before rename can leave only an unrelated temporary file.
	if err := os.WriteFile(filepath.Join(dir, ".vault-interrupted"), []byte(`{"truncated":`), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, _ := New(path)
	if err := reopened.Unlock("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.Resolve("provider/key"); err != nil || got != "committed-secret" {
		t.Fatalf("resolved=%q, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v, err=%v", info.Mode().Perm(), err)
	}
}

func TestVaultChangePasswordReencryptsSecretsAndRejectsOldPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.vault")
	v, _ := New(path)
	oldPass := "correct horse battery staple"
	newPass := "new super secure master password"

	if err := v.Initialize(oldPass); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("provider/groq/api-key", "super-secret-token"); err != nil {
		t.Fatal(err)
	}

	// Change password with invalid old password fails
	if err := v.ChangePassword("wrong old password", newPass); err != ErrInvalidPassword {
		t.Fatalf("expected ErrInvalidPassword, got: %v", err)
	}

	// Change password with invalid new password fails
	if err := v.ChangePassword(oldPass, "short"); err == nil {
		t.Fatal("expected error for short new password, got nil")
	}

	// Successful re-key
	if err := v.ChangePassword(oldPass, newPass); err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Secret is still resolvable while unlocked
	got, err := v.Resolve("provider/groq/api-key")
	if err != nil || got != "super-secret-token" {
		t.Fatalf("resolve after rekey = %q, %v", got, err)
	}

	// Lock and verify old password can no longer unlock
	v.Lock()
	if err := v.Unlock(oldPass); err != ErrInvalidPassword {
		t.Fatalf("unlock with old pass expected ErrInvalidPassword, got: %v", err)
	}

	// New password unlocks successfully
	if err := v.Unlock(newPass); err != nil {
		t.Fatalf("unlock with new pass failed: %v", err)
	}
	got, err = v.Resolve("provider/groq/api-key")
	if err != nil || got != "super-secret-token" {
		t.Fatalf("resolve after unlock with new pass = %q, %v", got, err)
	}
}

func TestHTTPRekeyEndpoint(t *testing.T) {
	v, _ := New(filepath.Join(t.TempDir(), "vault"))
	h := HTTP{Vault: v}.Handler()

	// Initialize
	req := httptest.NewRequest(http.MethodPost, "http://local/initialize", strings.NewReader(`{"password":"correct horse battery staple"}`))
	req.RemoteAddr = "127.0.0.1:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("init=%d %s", w.Code, w.Body.String())
	}

	// Add secret
	req = httptest.NewRequest(http.MethodPut, "http://local/secrets/provider%2Fgroq%2Fapi-key", strings.NewReader(`{"value":"hidden-value"}`))
	req.SetPathValue("name", "provider/groq/api-key")
	req.RemoteAddr = "127.0.0.1:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("put=%d %s", w.Code, w.Body.String())
	}

	// Rekey via HTTP
	rekeyBody := `{"old_password":"correct horse battery staple","new_password":"new super secure master password"}`
	req = httptest.NewRequest(http.MethodPost, "http://local/rekey", strings.NewReader(rekeyBody))
	req.RemoteAddr = "127.0.0.1:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rekey=%d %s", w.Code, w.Body.String())
	}

	// Verify status indicates initialized and unlocked
	var st Status
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Initialized || st.Locked || len(st.Secrets) != 1 {
		t.Fatalf("unexpected status after rekey: %+v", st)
	}
}

func TestHTTPIsWriteOnlyAndLocalOnly(t *testing.T) {
	v, _ := New(filepath.Join(t.TempDir(), "vault"))
	h := HTTP{Vault: v}.Handler()
	req := httptest.NewRequest(http.MethodPost, "http://local/initialize", strings.NewReader(`{"password":"correct horse battery staple"}`))
	req.RemoteAddr = "127.0.0.1:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("init=%d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodPut, "http://local/secrets/provider%2Fgroq%2Fapi-key", strings.NewReader(`{"value":"hidden-value"}`))
	req.SetPathValue("name", "provider/groq/api-key")
	req.RemoteAddr = "127.0.0.1:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("put=%d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "http://local/status", nil)
	req.RemoteAddr = "10.0.0.2:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("remote=%d", w.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "http://local/status", nil)
	req.RemoteAddr = "127.0.0.1:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if strings.Contains(w.Body.String(), "hidden-value") {
		t.Fatal("status leaked value")
	}

	for _, origin := range []string{"http://local.evil", "://broken"} {
		req = httptest.NewRequest(http.MethodGet, "http://local/status", nil)
		req.RemoteAddr = "[::1]:1"
		req.Header.Set("Origin", origin)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("origin %q status=%d", origin, w.Code)
		}
	}
}

func TestHTTPInternalErrorRedactionAndValidationMapping(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, _ := New(vaultPath)
	h := HTTP{Vault: v}.Handler()

	// 1. Password validation error (< 12 chars) -> 400 invalid_request
	req := httptest.NewRequest(http.MethodPost, "http://local/initialize", strings.NewReader(`{"password":"short"}`))
	req.RemoteAddr = "127.0.0.1:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("short pass status=%d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_request") || !strings.Contains(w.Body.String(), "12 to 1024") {
		t.Fatalf("unexpected body for short pass: %s", w.Body.String())
	}

	// Initialize vault properly
	req = httptest.NewRequest(http.MethodPost, "http://local/initialize", strings.NewReader(`{"password":"correct horse battery staple"}`))
	req.RemoteAddr = "127.0.0.1:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("init status=%d %s", w.Code, w.Body.String())
	}

	// 2. Secret name validation error (empty or invalid characters) -> 400 invalid_request
	req = httptest.NewRequest(http.MethodPut, "http://local/secrets/%00", strings.NewReader(`{"value":"foo"}`))
	req.SetPathValue("name", "\x00")
	req.RemoteAddr = "127.0.0.1:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid secret name status=%d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_secret_name") && !strings.Contains(w.Body.String(), "invalid_request") {
		t.Fatalf("unexpected body for invalid secret name: %s", w.Body.String())
	}

	// 3. Corrupt vault file on disk -> Unlock returns 500 internal_error with redacted message (no path leak)
	if err := os.WriteFile(vaultPath, []byte(`{invalid-json`), 0600); err != nil {
		t.Fatal(err)
	}
	// Lock vault first so next operation tries to unlock/read file
	v.Lock()
	req = httptest.NewRequest(http.MethodPost, "http://local/unlock", strings.NewReader(`{"password":"correct horse battery staple"}`))
	req.RemoteAddr = "127.0.0.1:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("corrupt vault status=%d, want 500", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "internal_error") || !strings.Contains(body, "internal vault operation failed") {
		t.Fatalf("expected redacted internal error, got: %s", body)
	}
	if strings.Contains(body, vaultPath) || strings.Contains(body, "vault.json") {
		t.Fatalf("internal error leaked file path: %s", body)
	}
}
