package secretvault

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type HTTP struct{ Vault *Vault }

func (h HTTP) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", h.status)
	mux.HandleFunc("POST /initialize", h.initialize)
	mux.HandleFunc("POST /unlock", h.unlock)
	mux.HandleFunc("POST /lock", h.lock)
	mux.HandleFunc("POST /rekey", h.rekey)
	mux.HandleFunc("PUT /secrets/{name}", h.put)
	mux.HandleFunc("DELETE /secrets/{name}", h.delete)
	return localOnly(mux)
}

type passwordRequest struct {
	Password string `json:"password"`
}
type rekeyRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}
type secretRequest struct {
	Value string `json:"value"`
}

func (h HTTP) status(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, h.Vault.Status())
}
func (h HTTP) initialize(w http.ResponseWriter, r *http.Request) {
	var q passwordRequest
	if !decode(w, r, &q) {
		return
	}
	if err := h.Vault.Initialize(q.Password); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusCreated, h.Vault.Status())
}
func (h HTTP) unlock(w http.ResponseWriter, r *http.Request) {
	var q passwordRequest
	if !decode(w, r, &q) {
		return
	}
	if err := h.Vault.Unlock(q.Password); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, h.Vault.Status())
}
func (h HTTP) lock(w http.ResponseWriter, _ *http.Request) {
	h.Vault.Lock()
	write(w, http.StatusOK, h.Vault.Status())
}
func (h HTTP) rekey(w http.ResponseWriter, r *http.Request) {
	var q rekeyRequest
	if !decode(w, r, &q) {
		return
	}
	if err := h.Vault.ChangePassword(q.OldPassword, q.NewPassword); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, h.Vault.Status())
}
func (h HTTP) put(w http.ResponseWriter, r *http.Request) {
	var q secretRequest
	if !decode(w, r, &q) {
		return
	}
	if err := h.Vault.Put(r.PathValue("name"), q.Value); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusNoContent, nil)
}
func (h HTTP) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.Vault.Delete(r.PathValue("name")); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusNoContent, nil)
}
func decode(w http.ResponseWriter, r *http.Request, d any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 20<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(d); err != nil {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "invalid request body"}})
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "request must contain one JSON value"}})
		return false
	}
	return true
}
func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "invalid_request"
	message := err.Error()

	if errors.Is(err, ErrLocked) {
		status = http.StatusLocked
		code = "vault_locked"
	} else if errors.Is(err, ErrInvalidPassword) {
		status = http.StatusUnauthorized
		code = "invalid_password"
		message = "invalid vault password"
	} else if errors.Is(err, ErrUninitialized) {
		status = http.StatusConflict
		code = "vault_uninitialized"
	} else if errors.Is(err, ErrInitialized) {
		status = http.StatusConflict
		code = "vault_initialized"
	} else if errors.Is(err, os.ErrNotExist) {
		status = http.StatusNotFound
		code = "not_found"
	} else if errors.Is(err, ErrInvalidPasswordLength) || errors.Is(err, ErrInvalidSecretName) || errors.Is(err, ErrInvalidSecretValue) {
		status = http.StatusBadRequest
		code = "invalid_request"
	} else {
		status = http.StatusInternalServerError
		code = "internal_error"
		message = "internal vault operation failed"
	}
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = strings.Trim(r.RemoteAddr, "[]")
		}
		ip := net.ParseIP(host)
		if host != "" && (ip == nil || !ip.IsLoopback()) {
			write(w, http.StatusForbidden, map[string]any{"error": map[string]string{"code": "local_only", "message": "credential vault is available only from localhost"}})
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, parseErr := url.Parse(origin)
			if parseErr != nil || u.Scheme == "" || !strings.EqualFold(u.Host, r.Host) {
				write(w, http.StatusForbidden, map[string]any{"error": map[string]string{"code": "origin_rejected", "message": "cross-origin vault request rejected"}})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
