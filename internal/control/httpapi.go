package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

const maxSubmitBodyBytes = domain.MaxControlPayloadBytes

// API is the mutable Control API surface for Slice B. It accepts commands and
// external events into durable inboxes; kernel processors own effects.
//
// Auth and local bind remain deployment concerns. This package never elevates
// untrusted content into policy and never mutates canonical domain state
// beyond inbox persistence.
type API struct {
	Commands  *CommandInbox
	Events    *ExternalEventInbox
	Clock     source.Clock
	IDs       source.IDGenerator
	ActorType domain.ActorType
	// DefaultActorID is used when the request body omits actor_id. Production
	// deployments should still authenticate the caller and override this.
	DefaultActorID string
	// DefaultSource labels external events submitted through this API.
	DefaultSource string
}

func NewAPI(commands *CommandInbox, events *ExternalEventInbox, clock source.Clock, ids source.IDGenerator) (*API, error) {
	if commands == nil || events == nil || clock == nil || ids == nil {
		return nil, errors.New("control API requires command inbox, event inbox, clock, and ID generator")
	}
	return &API{
		Commands:       commands,
		Events:         events,
		Clock:          clock,
		IDs:            ids,
		ActorType:      domain.ActorOperator,
		DefaultActorID: "operator_local",
		DefaultSource:  "control-api",
	}, nil
}

// Handler returns routes for command/event submit and receipt lookup.
// Mount under /v1 with http.StripPrefix when desired.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /commands", a.handleSubmitCommand)
	mux.HandleFunc("GET /commands/{commandID}", a.handleGetCommand)
	mux.HandleFunc("GET /commands/{commandID}/receipt", a.handleGetCommandReceipt)
	mux.HandleFunc("POST /external-events", a.handleSubmitExternalEvent)
	mux.HandleFunc("GET /external-events/{eventID}", a.handleGetExternalEvent)
	mux.HandleFunc("GET /external-events/{eventID}/disposition", a.handleGetExternalEventDisposition)
	return mux
}

type commandSubmitRequest struct {
	SchemaVersion    int                        `json:"schema_version"`
	CommandID        domain.CommandID           `json:"command_id"`
	IdempotencyKey   domain.IdempotencyKey      `json:"idempotency_key"`
	ActorType        domain.ActorType           `json:"actor_type"`
	ActorID          string                     `json:"actor_id"`
	Kind             domain.OperatorCommandKind `json:"kind"`
	Target           domain.CommandTarget       `json:"target"`
	ExpectedRevision *uint64                    `json:"expected_revision,omitempty"`
	Reason           string                     `json:"reason"`
	SubmittedAt      *time.Time                 `json:"submitted_at,omitempty"`
}

type commandSubmitResponse struct {
	SchemaVersion int                   `json:"schema_version"`
	CommandID     domain.CommandID      `json:"command_id"`
	Receipt       domain.CommandReceipt `json:"receipt"`
	// Accepted means the command was durably received by the inbox. Effect
	// confirmation requires consulting the receipt state or inspection API.
	Accepted bool `json:"accepted"`
}

type externalEventSubmitRequest struct {
	SchemaVersion      int                     `json:"schema_version"`
	EventID            domain.ExternalEventID  `json:"event_id"`
	DeduplicationKey   string                  `json:"deduplication_key"`
	Source             string                  `json:"source"`
	SourceActorID      string                  `json:"source_actor_id"`
	Kind               domain.ExternalEventKind `json:"kind"`
	MissionID          domain.MissionID        `json:"mission_id,omitempty"`
	CorrelationID      string                  `json:"correlation_id,omitempty"`
	TransportMessageID string                  `json:"transport_message_id,omitempty"`
	Content            domain.ExternalContent  `json:"content"`
	ReceivedAt         *time.Time              `json:"received_at,omitempty"`
}

type externalEventSubmitResponse struct {
	SchemaVersion int                            `json:"schema_version"`
	EventID       domain.ExternalEventID         `json:"event_id"`
	Disposition   domain.ExternalEventDisposition `json:"disposition"`
	// Accepted means the stimulus was durably received. Kernel disposition may
	// later mark it applied, rejected, or ignored.
	Accepted bool `json:"accepted"`
}

func (a *API) handleSubmitCommand(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req commandSubmitRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	command := domain.OperatorCommand{
		SchemaVersion:    req.SchemaVersion,
		ID:               req.CommandID,
		IdempotencyKey:   req.IdempotencyKey,
		ActorType:        req.ActorType,
		ActorID:          strings.TrimSpace(req.ActorID),
		Kind:             req.Kind,
		Target:           req.Target,
		ExpectedRevision: req.ExpectedRevision,
		Reason:           strings.TrimSpace(req.Reason),
	}
	if req.SubmittedAt != nil {
		command.SubmittedAt = req.SubmittedAt.UTC()
	}
	if command.ActorType == "" {
		command.ActorType = a.ActorType
	}
	if command.ActorID == "" {
		command.ActorID = a.DefaultActorID
	}
	// Retries that keep only the idempotency key must reuse the durable identity
	// instead of minting a new command ID that would conflict with the key.
	if command.ID == "" && command.IdempotencyKey != "" {
		if existing, lookupErr := a.Commands.CommandByIdempotency(command.IdempotencyKey); lookupErr == nil {
			command.ID = existing.ID
			if command.SubmittedAt.IsZero() {
				command.SubmittedAt = existing.SubmittedAt
			}
		} else if lookupErr != nil && !errors.Is(lookupErr, port.ErrNotFound) {
			writeAPIError(w, mapStoreError(lookupErr, "command"))
			return
		}
	}
	filled, err := ensureCommandIdentity(command, a.Clock, a.IDs)
	if err != nil {
		writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "identity_failed", message: "could not assign command identity"})
		return
	}
	receipt, err := a.Commands.SubmitCommand(filled)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "command"))
		return
	}
	writeJSON(w, http.StatusAccepted, commandSubmitResponse{
		SchemaVersion: domain.SchemaVersionV1,
		CommandID:     filled.ID,
		Receipt:       receipt,
		Accepted:      true,
	})
}

func (a *API) handleGetCommand(w http.ResponseWriter, r *http.Request) {
	id := domain.CommandID(r.PathValue("commandID"))
	command, err := a.Commands.Command(id)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "command"))
		return
	}
	receipt, err := a.Commands.CommandReceipt(id)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "command"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"command":        command,
		"receipt":        receipt,
	})
}

func (a *API) handleGetCommandReceipt(w http.ResponseWriter, r *http.Request) {
	id := domain.CommandID(r.PathValue("commandID"))
	receipt, err := a.Commands.CommandReceipt(id)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "command"))
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func (a *API) handleSubmitExternalEvent(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req externalEventSubmitRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	event := domain.ExternalEvent{
		SchemaVersion:      req.SchemaVersion,
		ID:                 req.EventID,
		DeduplicationKey:   strings.TrimSpace(req.DeduplicationKey),
		Source:             strings.TrimSpace(req.Source),
		SourceActorID:      strings.TrimSpace(req.SourceActorID),
		Kind:               req.Kind,
		MissionID:          req.MissionID,
		CorrelationID:      strings.TrimSpace(req.CorrelationID),
		TransportMessageID: strings.TrimSpace(req.TransportMessageID),
		Content:            req.Content,
	}
	if req.ReceivedAt != nil {
		event.ReceivedAt = req.ReceivedAt.UTC()
	}
	if event.Source == "" {
		event.Source = a.DefaultSource
	}
	if event.SourceActorID == "" {
		event.SourceActorID = a.DefaultActorID
	}
	// Retries that keep only the deduplication key must reuse the durable identity
	// instead of minting a new event ID that would conflict with the key.
	if event.ID == "" && event.DeduplicationKey != "" {
		if existing, lookupErr := a.Events.ExternalEventByDeduplicationKey(event.DeduplicationKey); lookupErr == nil {
			event.ID = existing.ID
			if event.ReceivedAt.IsZero() {
				event.ReceivedAt = existing.ReceivedAt
			}
		} else if lookupErr != nil && !errors.Is(lookupErr, port.ErrNotFound) {
			writeAPIError(w, mapStoreError(lookupErr, "external_event"))
			return
		}
	}
	filled, err := ensureExternalEventIdentity(event, a.Clock, a.IDs)
	if err != nil {
		writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "identity_failed", message: "could not assign external event identity"})
		return
	}
	disposition, err := a.Events.SubmitExternalEvent(filled)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "external_event"))
		return
	}
	writeJSON(w, http.StatusAccepted, externalEventSubmitResponse{
		SchemaVersion: domain.SchemaVersionV1,
		EventID:       filled.ID,
		Disposition:   disposition,
		Accepted:      true,
	})
}

func (a *API) handleGetExternalEvent(w http.ResponseWriter, r *http.Request) {
	id := domain.ExternalEventID(r.PathValue("eventID"))
	event, err := a.Events.ExternalEvent(id)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "external_event"))
		return
	}
	disposition, err := a.Events.ExternalEventDisposition(id)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "external_event"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"event":          event,
		"disposition":    disposition,
	})
}

func (a *API) handleGetExternalEventDisposition(w http.ResponseWriter, r *http.Request) {
	id := domain.ExternalEventID(r.PathValue("eventID"))
	disposition, err := a.Events.ExternalEventDisposition(id)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "external_event"))
		return
	}
	writeJSON(w, http.StatusOK, disposition)
}

type apiError struct {
	status  int
	code    string
	message string
}

func (e apiError) Error() string {
	return e.message
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

func writeAPIError(w http.ResponseWriter, err error) {
	var apiErr apiError
	if errors.As(err, &apiErr) {
		writeJSON(w, apiErr.status, errorBody{Error: errorDetail{Code: apiErr.code, Message: apiErr.message}})
		return
	}
	writeJSON(w, http.StatusInternalServerError, errorBody{Error: errorDetail{Code: "internal_error", Message: "request failed"}})
}

func mapStoreError(err error, resource string) error {
	switch {
	case errors.Is(err, port.ErrNotFound):
		return apiError{status: http.StatusNotFound, code: "not_found", message: resource + " not found"}
	case errors.Is(err, port.ErrConflict):
		return apiError{status: http.StatusConflict, code: "conflict", message: resource + " conflict"}
	case isValidationError(err):
		return apiError{status: http.StatusBadRequest, code: "invalid_request", message: sanitizeValidationMessage(err)}
	default:
		return apiError{status: http.StatusInternalServerError, code: "internal_error", message: "request failed"}
	}
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "validate") ||
		strings.Contains(msg, "incomplete") ||
		strings.Contains(msg, "unknown") ||
		strings.Contains(msg, "requires") ||
		strings.Contains(msg, "must not") ||
		strings.Contains(msg, "must be") ||
		strings.Contains(msg, "exceeds") ||
		strings.Contains(msg, "unsupported")
}

func sanitizeValidationMessage(err error) string {
	msg := err.Error()
	// Keep messages short and free of nested stack noise.
	if idx := strings.Index(msg, ": "); idx >= 0 && idx+2 < len(msg) {
		// Prefer the leaf validation reason when present.
		leaf := strings.TrimSpace(msg[idx+2:])
		if leaf != "" && !strings.Contains(leaf, "\n") {
			return leaf
		}
	}
	if len(msg) > 200 {
		return msg[:200]
	}
	return msg
}

func readLimitedJSON(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, apiError{status: http.StatusBadRequest, code: "invalid_body", message: "request body is required"}
	}
	defer r.Body.Close()
	limited := io.LimitReader(r.Body, maxSubmitBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, apiError{status: http.StatusBadRequest, code: "invalid_body", message: "could not read request body"}
	}
	if len(body) == 0 {
		return nil, apiError{status: http.StatusBadRequest, code: "invalid_body", message: "request body is required"}
	}
	if len(body) > maxSubmitBodyBytes {
		return nil, apiError{status: http.StatusRequestEntityTooLarge, code: "body_too_large", message: fmt.Sprintf("request body exceeds %d bytes", maxSubmitBodyBytes)}
	}
	return body, nil
}

func decodeStrictJSON(body []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return apiError{status: http.StatusBadRequest, code: "invalid_json", message: "request body must be strict JSON matching the schema"}
	}
	// Reject trailing data after the first value.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return apiError{status: http.StatusBadRequest, code: "invalid_json", message: "request body must contain a single JSON value"}
	}
	return nil
}
