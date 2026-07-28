package secretvault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
