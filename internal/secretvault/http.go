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
	"time"
)

type HTTP struct{ Vault *Vault }

func (h HTTP) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", h.status)
	mux.HandleFunc("GET /audit", h.auditLog)
	mux.HandleFunc("POST /initialize", h.initialize)
	mux.HandleFunc("POST /unlock", h.unlock)
	mux.HandleFunc("POST /lock", h.lock)
	mux.HandleFunc("POST /close", h.close)
	mux.HandleFunc("POST /rekey", h.rekey)
	mux.HandleFunc("POST /export", h.export)
	mux.HandleFunc("POST /import", h.importVault)
	mux.HandleFunc("GET /stats", h.stats)
	mux.HandleFunc("GET /secrets", h.listSecrets)
	mux.HandleFunc("GET /secrets/{name}/metadata", h.secretMetadata)
	mux.HandleFunc("GET /secrets/{name}", h.getSecret)
	mux.HandleFunc("POST /secrets/{name}/rotate", h.rotateSecret)
	mux.HandleFunc("POST /purge-expired", h.purgeExpired)
	mux.HandleFunc("DELETE /secrets", h.deleteAllSecrets)
	mux.HandleFunc("PUT /secrets/{name}", h.put)
	mux.HandleFunc("DELETE /secrets/{name}", h.delete)
	mux.HandleFunc("POST /resolve", h.resolveBatch)
	return localOnly(mux)
}

type passwordRequest struct {
	Password string `json:"password"`
}
type rekeyRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}
type exportRequest struct {
	BackupPath     string `json:"backup_path"`
	BackupPassword string `json:"backup_password"`
}
type importRequest struct {
	BackupPath     string `json:"backup_path"`
	BackupPassword string `json:"backup_password"`
	Mode           string `json:"mode,omitempty"`
}
type secretRequest struct {
	Value string `json:"value"`
	TTL   string `json:"ttl,omitempty"`
}

type resolveRequest struct {
	Names []string `json:"names"`
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
func (h HTTP) close(w http.ResponseWriter, _ *http.Request) {
	h.Vault.Close()
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
func (h HTTP) export(w http.ResponseWriter, r *http.Request) {
	var q exportRequest
	if !decode(w, r, &q) {
		return
	}
	if err := h.Vault.Export(q.BackupPath, q.BackupPassword); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, h.Vault.Status())
}
func (h HTTP) importVault(w http.ResponseWriter, r *http.Request) {
	var q importRequest
	if !decode(w, r, &q) {
		return
	}
	var mode ImportMode
	switch strings.ToLower(strings.TrimSpace(q.Mode)) {
	case "", "fail":
		mode = ImportModeFail
	case "skip":
		mode = ImportModeSkip
	case "overwrite":
		mode = ImportModeOverwrite
	default:
		writeErr(w, ErrInvalidImportMode)
		return
	}
	res, err := h.Vault.ImportWithOptions(q.BackupPath, q.BackupPassword, ImportOptions{Mode: mode})
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, res)
}
func (h HTTP) auditLog(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, h.Vault.AuditLog())
}
func (h HTTP) stats(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, h.Vault.Stats())
}
func (h HTTP) listSecrets(w http.ResponseWriter, _ *http.Request) {
	entries, err := h.Vault.ListSecrets()
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"secrets": entries})
}
func (h HTTP) secretMetadata(w http.ResponseWriter, r *http.Request) {
	entry, err := h.Vault.SecretMetadata(r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, entry)
}
func (h HTTP) getSecret(w http.ResponseWriter, r *http.Request) {
	val, err := h.Vault.Resolve(r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]string{"value": val})
}
func (h HTTP) rotateSecret(w http.ResponseWriter, r *http.Request) {
	var q secretRequest
	if !decode(w, r, &q) {
		return
	}
	ttlStr := strings.TrimSpace(q.TTL)
	if ttlStr == "" {
		ttlStr = strings.TrimSpace(r.URL.Query().Get("ttl"))
	}
	var err error
	if ttlStr != "" {
		dur, parseErr := time.ParseDuration(ttlStr)
		if parseErr != nil {
			writeErr(w, ErrInvalidTTL)
			return
		}
		err = h.Vault.RotateWithTTL(r.PathValue("name"), q.Value, dur)
	} else {
		err = h.Vault.Rotate(r.PathValue("name"), q.Value)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusNoContent, nil)
}
func (h HTTP) put(w http.ResponseWriter, r *http.Request) {
	var q secretRequest
	if !decode(w, r, &q) {
		return
	}
	ttlStr := strings.TrimSpace(q.TTL)
	if ttlStr == "" {
		ttlStr = strings.TrimSpace(r.URL.Query().Get("ttl"))
	}
	var err error
	if ttlStr != "" {
		dur, parseErr := time.ParseDuration(ttlStr)
		if parseErr != nil {
			writeErr(w, ErrInvalidTTL)
			return
		}
		err = h.Vault.PutWithTTL(r.PathValue("name"), q.Value, dur)
	} else {
		err = h.Vault.Put(r.PathValue("name"), q.Value)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusNoContent, nil)
}
func (h HTTP) purgeExpired(w http.ResponseWriter, _ *http.Request) {
	removed, err := h.Vault.PurgeExpired()
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"purged": removed})
}
func (h HTTP) deleteAllSecrets(w http.ResponseWriter, _ *http.Request) {
	deleted, err := h.Vault.DeleteAll()
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"deleted": deleted})
}
func (h HTTP) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.Vault.Delete(r.PathValue("name")); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusNoContent, nil)
}
func (h HTTP) resolveBatch(w http.ResponseWriter, r *http.Request) {
	var q resolveRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Names) == 0 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "names list is required"}})
		return
	}
	if len(q.Names) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "maximum 100 secret names per resolve batch"}})
		return
	}
	results := h.Vault.ResolveAll(q.Names)
	write(w, http.StatusOK, results)
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
	} else if errors.Is(err, ErrSecretExpired) {
		status = http.StatusGone
		code = "secret_expired"
	} else if errors.Is(err, ErrInvalidPasswordLength) || errors.Is(err, ErrInvalidSecretName) || errors.Is(err, ErrInvalidSecretValue) || errors.Is(err, ErrInvalidBackupPath) || errors.Is(err, ErrInvalidBackupFormat) || errors.Is(err, ErrInvalidImportMode) || errors.Is(err, ErrInvalidTTL) {
		status = http.StatusBadRequest
		code = "invalid_request"
	} else if errors.Is(err, ErrImportConflict) {
		status = http.StatusConflict
		code = "import_conflict"
	} else if errors.Is(err, ErrAccountLockedOut) {
		status = http.StatusTooManyRequests
		code = "rate_limited"
		message = "vault is locked out due to consecutive failed attempts"
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
