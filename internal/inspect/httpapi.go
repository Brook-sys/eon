package inspect

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// API is a read-only Control API surface for Slice A.
// It never mutates store state and never executes capabilities.
type API struct {
	Projector *Projector
	// Bind note: production deployments should still enforce auth and local bind.
}

func NewAPI(projector *Projector) (*API, error) {
	if projector == nil {
		return nil, errors.New("inspect API requires projector")
	}
	return &API{Projector: projector}, nil
}

// Handler returns an http.Handler mounted at the root of the control API tree.
// Paths are relative; mount under /v1 with http.StripPrefix when desired.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("GET /version", a.handleVersion)
	mux.HandleFunc("GET /overview", a.handleOverview)
	mux.HandleFunc("GET /missions/{missionID}", a.handleMission)
	mux.HandleFunc("GET /missions/{missionID}/operations", a.handleMissionOperations)
	mux.HandleFunc("GET /operations/{operationID}", a.handleOperation)
	mux.HandleFunc("GET /commits/{commitID}", a.handleCommit)
	mux.HandleFunc("GET /commands/{commandID}", a.handleCommand)
	mux.HandleFunc("GET /events", a.handleEvents)
	mux.HandleFunc("GET /events/{eventID}", a.handleEvent)
	mux.HandleFunc("GET /events/stream", a.handleEventStream)
	return mux
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	health, err := a.Projector.HealthProbe(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "store_unreachable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, health)
}

func (a *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"runtime":        a.Projector.Runtime,
		"generated_at":   a.Projector.Clock().UTC().Format(time.RFC3339Nano),
	})
}

func (a *API) handleOverview(w http.ResponseWriter, r *http.Request) {
	missionID := domain.MissionID(strings.TrimSpace(r.URL.Query().Get("mission_id")))
	overview, err := a.Projector.BuildOverview(r.Context(), missionID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (a *API) handleMission(w http.ResponseWriter, r *http.Request) {
	missionID := domain.MissionID(r.PathValue("missionID"))
	detail, err := a.Projector.MissionDetail(r.Context(), missionID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *API) handleMissionOperations(w http.ResponseWriter, r *http.Request) {
	missionID := domain.MissionID(r.PathValue("missionID"))
	detail, err := a.Projector.MissionDetail(r.Context(), missionID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version":     domain.SchemaVersionV1,
		"mission_id":         detail.MissionID,
		"active_revision_id": detail.ActiveRevisionID,
		"active_revision":    detail.ActiveRevision,
		"agenda":             detail.Agenda,
		"operations":         detail.Operations,
	})
}

func (a *API) handleOperation(w http.ResponseWriter, r *http.Request) {
	operationID := domain.OperationID(r.PathValue("operationID"))
	detail, err := a.Projector.OperationInspector(r.Context(), operationID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *API) handleCommit(w http.ResponseWriter, r *http.Request) {
	commitID := domain.CommitID(r.PathValue("commitID"))
	detail, err := a.Projector.CommitInspector(r.Context(), commitID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *API) handleCommand(w http.ResponseWriter, r *http.Request) {
	commandID := domain.CommandID(r.PathValue("commandID"))
	detail, err := a.Projector.CommandInspector(r.Context(), commandID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	after, err := parseUint64Default(q.Get("after_sequence"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_after_sequence", "after_sequence must be an unsigned integer")
		return
	}
	limit, err := parseIntDefault(q.Get("limit"), DefaultEventPageLimit)
	if err != nil || limit <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
		return
	}
	page, err := a.Projector.ListEvents(r.Context(), EventFilter{
		AfterSequence:   after,
		Limit:           limit,
		Kind:            q.Get("kind"),
		MissionRevision: domain.MissionRevisionID(q.Get("mission_revision_id")),
		InquiryID:       domain.InquiryID(q.Get("inquiry_id")),
		OperationID:     domain.OperationID(q.Get("operation_id")),
		CommitID:        domain.CommitID(q.Get("commit_id")),
	})
	if err != nil {
		if strings.Contains(err.Error(), "limit must be") {
			writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
			return
		}
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *API) handleEvent(w http.ResponseWriter, r *http.Request) {
	eventID := domain.EventID(r.PathValue("eventID"))
	event, err := a.Projector.GetEvent(r.Context(), eventID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, event)
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, port.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, port.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource conflict")
	default:
		// Do not leak internal error details beyond a stable code for unknown faults.
		writeError(w, http.StatusInternalServerError, "internal_error", "inspection failed")
	}
}

func parseUint64Default(raw string, fallback uint64) (uint64, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}

func parseIntDefault(raw string, fallback int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}
