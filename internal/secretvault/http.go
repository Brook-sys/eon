package secretvault

import (
	"encoding/json"
	"errors"
	"fmt"
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
	mux.HandleFunc("GET /audit/summary", h.auditSummary)
	mux.HandleFunc("POST /initialize", h.initialize)
	mux.HandleFunc("POST /unlock", h.unlock)
	mux.HandleFunc("POST /lock", h.lock)
	mux.HandleFunc("POST /close", h.close)
	mux.HandleFunc("POST /rekey", h.rekey)
	mux.HandleFunc("POST /export", h.export)
	mux.HandleFunc("POST /import", h.importVault)
	mux.HandleFunc("GET /stats", h.stats)
	mux.HandleFunc("GET /secrets", h.listSecrets)
	mux.HandleFunc("POST /secrets/batch-purge-expired", h.batchPurgeExpired)
	mux.HandleFunc("POST /secrets/batch-delete", h.batchDelete)
	mux.HandleFunc("POST /secrets/batch-put", h.batchPut)
	mux.HandleFunc("POST /secrets/batch-rotate", h.batchRotate)
	mux.HandleFunc("POST /secrets/batch-metadata", h.batchMetadata)
	mux.HandleFunc("POST /secrets/batch-exists", h.batchExists)
	mux.HandleFunc("POST /secrets/batch-refresh", h.batchRefreshToken)
	mux.HandleFunc("POST /secrets/{name}/refresh", h.refreshToken)
	mux.HandleFunc("POST /secrets/batch-copy", h.batchCopy)
	mux.HandleFunc("GET /secrets/expiring-soon", h.expiringSoon)
	mux.HandleFunc("POST /secrets/bulk-touch", h.bulkTouch)
	mux.HandleFunc("POST /secrets/batch-expire-at", h.batchExpireAt)
	mux.HandleFunc("POST /secrets/{name}/expire-at", h.expireAtSecret)
	mux.HandleFunc("GET /secrets/{name}/metadata", h.secretMetadata)
	mux.HandleFunc("GET /secrets/{name}/exists", h.secretExists)
	mux.HandleFunc("GET /secrets/{name}", h.getSecret)
	mux.HandleFunc("POST /secrets/{name}/rotate", h.rotateSecret)
	mux.HandleFunc("POST /secrets/{name}/copy", h.copySecret)
	mux.HandleFunc("POST /secrets/{name}/rename", h.renameSecret)
	mux.HandleFunc("POST /purge-expired", h.purgeExpired)
	mux.HandleFunc("DELETE /secrets", h.deleteAllSecrets)
	mux.HandleFunc("PUT /secrets/{name}", h.put)
	mux.HandleFunc("DELETE /secrets/{name}", h.delete)
	mux.HandleFunc("POST /resolve", h.resolveBatch)
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /secrets/{name}/history", h.secretHistory)
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

type batchDeleteRequest struct {
	Names []string `json:"names"`
}

type batchPutRequest struct {
	Items []BatchPutItem `json:"items"`
}

type batchRotateRequest struct {
	Items []BatchRotateItem `json:"items"`
}

type batchMetadataRequest struct {
	Names []string `json:"names"`
}

type batchExistsRequest struct {
	Names []string `json:"names"`
}

type bulkTouchRequest struct {
	Items []BulkTouchItem `json:"items"`
}

type expireAtRequest struct {
	ExpiresAt time.Time `json:"expires_at"`
}

type batchExpireAtRequest struct {
	Items []BatchExpireAtItem `json:"items"`
}

type copyRequest struct {
	Destination string `json:"destination"`
}

type batchCopyRequest struct {
	Items []BatchCopyItem `json:"items"`
}

type tokenRefreshRequest struct {
	NewValue string `json:"new_value"`
	TTL      string `json:"ttl,omitempty"`
}

type batchTokenRefreshRequest struct {
	Items []TokenRefreshItem `json:"items"`
}

type renameRequest struct {
	Destination string `json:"destination"`
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
func (h HTTP) auditLog(w http.ResponseWriter, r *http.Request) {
	filter := AuditFilter{
		Action: strings.TrimSpace(r.URL.Query().Get("action")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
	}
	if since := strings.TrimSpace(r.URL.Query().Get("since")); since != "" {
		parsed, err := time.Parse(time.RFC3339, since)
		if err != nil {
			write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "since must be RFC3339"}})
			return
		}
		filter.Since = parsed
	}
	if until := strings.TrimSpace(r.URL.Query().Get("until")); until != "" {
		parsed, err := time.Parse(time.RFC3339, until)
		if err != nil {
			write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "until must be RFC3339"}})
			return
		}
		filter.Until = parsed
	}
	if limitStr := strings.TrimSpace(r.URL.Query().Get("limit")); limitStr != "" {
		var lim int
		if _, err := fmt.Sscanf(limitStr, "%d", &lim); err != nil || lim < 1 {
			write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "limit must be a positive integer"}})
			return
		}
		if lim > 1000 {
			lim = 1000
		}
		filter.Limit = lim
	}
	write(w, http.StatusOK, h.Vault.AuditLogFiltered(filter))
}

// auditSummary returns aggregate counters (total/matched events, per-action
// and per-status distribution, distinct secret count, first/last timestamps)
// for audit events matching the given query filters. It never exposes secret
// values and works in any lock state. The `limit` parameter is ignored for
// aggregation, keeping the summary complete.
func (h HTTP) auditSummary(w http.ResponseWriter, r *http.Request) {
	filter := AuditFilter{
		Action: strings.TrimSpace(r.URL.Query().Get("action")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
	}
	if since := strings.TrimSpace(r.URL.Query().Get("since")); since != "" {
		parsed, err := time.Parse(time.RFC3339, since)
		if err != nil {
			write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "since must be RFC3339"}})
			return
		}
		filter.Since = parsed
	}
	if until := strings.TrimSpace(r.URL.Query().Get("until")); until != "" {
		parsed, err := time.Parse(time.RFC3339, until)
		if err != nil {
			write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "until must be RFC3339"}})
			return
		}
		filter.Until = parsed
	}
	write(w, http.StatusOK, h.Vault.AuditSummary(filter))
}
func (h HTTP) stats(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, h.Vault.Stats())
}
func (h HTTP) listSecrets(w http.ResponseWriter, r *http.Request) {
	entries, err := h.Vault.SearchSecrets(strings.TrimSpace(r.URL.Query().Get("prefix")), strings.TrimSpace(r.URL.Query().Get("search")))
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
func (h HTTP) secretExists(w http.ResponseWriter, r *http.Request) {
	exists, err := h.Vault.Exists(r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"name": r.PathValue("name"), "exists": exists})
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
func (h HTTP) copySecret(w http.ResponseWriter, r *http.Request) {
	var q copyRequest
	if !decode(w, r, &q) {
		return
	}
	if q.Destination == "" {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "destination is required"}})
		return
	}
	if err := h.Vault.CopySecret(r.PathValue("name"), q.Destination); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusCreated, map[string]any{"status": "created", "source": r.PathValue("name"), "destination": q.Destination})
}
func (h HTTP) renameSecret(w http.ResponseWriter, r *http.Request) {
	var q renameRequest
	if !decode(w, r, &q) {
		return
	}
	if q.Destination == "" {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "destination is required"}})
		return
	}
	if err := h.Vault.RenameSecret(r.PathValue("name"), q.Destination); err != nil {
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
type batchPurgeRequest struct {
	Names []string `json:"names"`
}

func (h HTTP) batchPurgeExpired(w http.ResponseWriter, r *http.Request) {
	var q batchPurgeRequest
	if !decode(w, r, &q) {
		return
	}
	result, err := h.Vault.BatchPurgeExpired(q.Names)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, result)
}

func (h HTTP) batchDelete(w http.ResponseWriter, r *http.Request) {
	var q batchDeleteRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Names) == 0 || len(q.Names) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "names must contain 1 to 100 secret names"}})
		return
	}
	result, err := h.Vault.BatchDelete(q.Names)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, result)
}
func (h HTTP) batchPut(w http.ResponseWriter, r *http.Request) {
	var q batchPutRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Items) == 0 || len(q.Items) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "items must contain 1 to 100 secret items"}})
		return
	}
	result, err := h.Vault.BatchPut(q.Items)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, result)
}
func (h HTTP) batchRotate(w http.ResponseWriter, r *http.Request) {
	var q batchRotateRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Items) == 0 || len(q.Items) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "items must contain 1 to 100 rotate items"}})
		return
	}
	result, err := h.Vault.BatchRotate(q.Items)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, result)
}
func (h HTTP) batchMetadata(w http.ResponseWriter, r *http.Request) {
	var q batchMetadataRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Names) == 0 || len(q.Names) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "names must contain 1 to 100 secret names"}})
		return
	}
	results, err := h.Vault.BatchMetadata(q.Names)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"metadata": results})
}

func (h HTTP) batchExists(w http.ResponseWriter, r *http.Request) {
	var q batchExistsRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Names) == 0 || len(q.Names) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "names must contain 1 to 100 secret names"}})
		return
	}
	results, err := h.Vault.BatchExists(q.Names)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"results": results})
}
func (h HTTP) refreshToken(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var q tokenRefreshRequest
	if !decode(w, r, &q) {
		return
	}
	var dur time.Duration
	if q.TTL != "" {
		var err error
		dur, err = time.ParseDuration(q.TTL)
		if err != nil {
			write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "invalid ttl format"}})
			return
		}
	}
	if err := h.Vault.RefreshToken(name, q.NewValue, dur); err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h HTTP) batchRefreshToken(w http.ResponseWriter, r *http.Request) {
	var q batchTokenRefreshRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Items) == 0 || len(q.Items) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "items must contain 1 to 100 refresh items"}})
		return
	}
	result, err := h.Vault.BatchRefreshToken(q.Items)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, result)
}

func (h HTTP) batchCopy(w http.ResponseWriter, r *http.Request) {
	var q batchCopyRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Items) == 0 || len(q.Items) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "items must contain 1 to 100 copy items"}})
		return
	}
	result, err := h.Vault.BatchCopy(q.Items)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, result)
}
func (h HTTP) expiringSoon(w http.ResponseWriter, r *http.Request) {
	window := 24 * time.Hour
	if raw := strings.TrimSpace(r.URL.Query().Get("window")); raw != "" {
		dur, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			writeErr(w, ErrInvalidTTL)
			return
		}
		window = dur
	}
	items, err := h.Vault.ExpiringSoon(window)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"window": window.String(), "items": items})
}
func (h HTTP) bulkTouch(w http.ResponseWriter, r *http.Request) {
	var q bulkTouchRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Items) == 0 || len(q.Items) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "items must contain 1 to 100 touch items"}})
		return
	}
	result, err := h.Vault.BulkTouch(q.Items)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, result)
}
func (h HTTP) batchExpireAt(w http.ResponseWriter, r *http.Request) {
	var q batchExpireAtRequest
	if !decode(w, r, &q) {
		return
	}
	if len(q.Items) == 0 || len(q.Items) > 100 {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "items must contain 1 to 100 expire-at items"}})
		return
	}
	result, err := h.Vault.BatchExpireAt(q.Items)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, result)
}

func (h HTTP) expireAtSecret(w http.ResponseWriter, r *http.Request) {
	var q expireAtRequest
	if !decode(w, r, &q) {
		return
	}
	if err := h.Vault.ExpireAt(r.PathValue("name"), q.ExpiresAt); err != nil {
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
func (h HTTP) health(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, h.Vault.Health())
}
func (h HTTP) secretHistory(w http.ResponseWriter, r *http.Request) {
	events, err := h.Vault.SecretHistory(r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"history": events})
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
