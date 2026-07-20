package control

import (
	"bytes"
	"context"
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

// ConfigDraftValidator marks OPEN drafts VALIDATED (or REJECTED) without
// elevating payload text into authority. Implemented by kernel.ConfigApplier.
type ConfigDraftValidator interface {
	ValidateDraft(context.Context, domain.ConfigDraftID) (domain.ConfigImpactPreview, domain.ConfigDiff, error)
}

// ConfigDraftApplicator promotes a VALIDATED draft to an immutable revision.
// Implemented by kernel.ConfigApplier.
type ConfigDraftApplicator interface {
	ApplyDraft(context.Context, domain.ConfigDraftID) (domain.ConfigRevision, domain.ConfigApplyReceipt, error)
}

// ConfigRevisionRollbacker re-applies an ancestral revision payload as a new
// forward revision (semantic rollback). Optional; without it the route 503s.
type ConfigRevisionRollbacker interface {
	RollbackToRevision(context.Context, domain.ConfigScope, domain.ConfigRevisionID, domain.ActorType, string, string) (domain.ConfigDraft, domain.ConfigRevision, domain.ConfigApplyReceipt, error)
}

// MissionAmendmentAcceptance is the durable outcome of FR-AUTH-004 accept.
// Mirrors mission.AmendmentAcceptance without forcing control clients to
// import the mission package for response decoding.
type MissionAmendmentAcceptance struct {
	Previous domain.MissionRevision            `json:"previous"`
	Accepted domain.MissionRevision            `json:"accepted"`
	Diff     domain.MissionDiff                `json:"diff"`
	Impact   domain.MissionImpactPreview       `json:"impact"`
	Report   domain.AgendaReconciliationReport `json:"report"`
}

// MissionAmendmentAcceptor installs an accepted UserAmendment (FR-AUTH-004).
// Implemented by mission.Acceptor via a thin adapter in bootstrap. Optional;
// without it POST .../accept 503s. Preview is pure and always available when
// the event store is wired.
type MissionAmendmentAcceptor interface {
	Accept(context.Context, domain.UserAmendment, string) (MissionAmendmentAcceptance, error)
}

// MissionAmendmentAcceptorFunc adapts a function to MissionAmendmentAcceptor.
type MissionAmendmentAcceptorFunc func(context.Context, domain.UserAmendment, string) (MissionAmendmentAcceptance, error)

func (f MissionAmendmentAcceptorFunc) Accept(ctx context.Context, amendment domain.UserAmendment, provenance string) (MissionAmendmentAcceptance, error) {
	return f(ctx, amendment, provenance)
}

// API is the mutable Control API surface for Slice B. It accepts commands and
// external events into durable inboxes; kernel processors own effects.
//
// Auth and local bind remain deployment concerns. This package never elevates
// untrusted content into policy and never mutates canonical domain state
// beyond inbox persistence, operator-authored config drafts, and explicit
// FR-AUTH-004 mission amendment acceptance through the wired acceptor.
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
	// ConfigValidate/ConfigApply are optional. Without them, draft create/list
	// and active-revision reads still work; validate/apply return 503.
	ConfigValidate ConfigDraftValidator
	ConfigApply    ConfigDraftApplicator
	// ConfigRollback is optional. Without it, POST .../revisions/rollback 503s.
	ConfigRollback ConfigRevisionRollbacker
	// MissionAccept is optional. Without it, POST /missions/amendments/accept 503s.
	// Preview remains available whenever the event store is wired.
	MissionAccept MissionAmendmentAcceptor
	// ModelPresets is an optional, startup-validated catalog. Presets are
	// read-only evidence-backed inputs; selecting one only creates a disabled
	// MODELS draft and never changes routing authority directly.
	ModelPresets *domain.ModelPresetCatalog
	// SemanticMemory atomically updates the current view and append-only audit log.
	SemanticMemory       *SemanticMemory
	SemanticMemoryReader port.MemoryReader
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
	mux.HandleFunc("GET /questions", a.handleListQuestions)
	mux.HandleFunc("GET /questions/{questionID}", a.handleGetQuestion)
	mux.HandleFunc("POST /questions/{questionID}/answers", a.handleSubmitQuestionAnswer)
	mux.HandleFunc("GET /config/drafts", a.handleListConfigDrafts)
	mux.HandleFunc("POST /config/drafts", a.handleCreateConfigDraft)
	mux.HandleFunc("GET /config/drafts/{draftID}", a.handleGetConfigDraft)
	mux.HandleFunc("POST /config/drafts/{draftID}/validate", a.handleValidateConfigDraft)
	mux.HandleFunc("POST /config/drafts/{draftID}/apply", a.handleApplyConfigDraft)
	mux.HandleFunc("GET /config/drafts/{draftID}/receipt", a.handleGetConfigApplyReceipt)
	mux.HandleFunc("GET /config/revisions/active", a.handleGetActiveConfigRevision)
	mux.HandleFunc("GET /config/revisions", a.handleListConfigRevisions)
	mux.HandleFunc("POST /config/revisions/rollback", a.handleRollbackConfigRevision)
	mux.HandleFunc("GET /model-presets", a.handleListModelPresets)
	mux.HandleFunc("GET /model-presets/{presetID}", a.handleGetModelPreset)
	mux.HandleFunc("POST /model-presets/{presetID}/drafts", a.handleCreateModelPresetDraft)
	mux.HandleFunc("POST /model-presets/{presetID}/enablement-preview", a.handlePreviewModelPresetEnablement)
	mux.HandleFunc("POST /model-presets/{presetID}/enable-drafts", a.handleCreateModelPresetEnableDraft)
	mux.HandleFunc("GET /missions/{missionID}/active", a.handleGetActiveMission)
	mux.HandleFunc("POST /missions/amendments/preview", a.handlePreviewMissionAmendment)
	mux.HandleFunc("POST /missions/amendments/accept", a.handleAcceptMissionAmendment)
	mux.HandleFunc("POST /memories", a.handleSubmitMemory)
	mux.HandleFunc("GET /memories", a.handleListMemories)
	mux.HandleFunc("DELETE /memories/{id}", a.handleDeleteMemory)
	return mux
}

func (a *API) modelPreset(id string) (domain.ModelPreset, bool) {
	if a.ModelPresets == nil {
		return domain.ModelPreset{}, false
	}
	for _, preset := range a.ModelPresets.Presets {
		if preset.ID == id {
			return preset, true
		}
	}
	return domain.ModelPreset{}, false
}

func (a *API) handleListModelPresets(w http.ResponseWriter, _ *http.Request) {
	if a.ModelPresets == nil {
		writeAPIError(w, apiError{status: http.StatusServiceUnavailable, code: "not_configured", message: "model preset catalog is not wired"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"catalog_schema": a.ModelPresets.Schema,
		"presets":        a.ModelPresets.Presets,
	})
}

func (a *API) handleGetModelPreset(w http.ResponseWriter, r *http.Request) {
	preset, ok := a.modelPreset(strings.TrimSpace(r.PathValue("presetID")))
	if !ok {
		writeAPIError(w, apiError{status: http.StatusNotFound, code: "not_found", message: "model preset not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": domain.SchemaVersionV1, "preset": preset})
}

type modelPresetDraftRequest struct {
	SchemaVersion   int                  `json:"schema_version"`
	DraftID         domain.ConfigDraftID `json:"draft_id,omitempty"`
	BasedOnRevision uint64               `json:"based_on_revision"`
	Version         string               `json:"version"`
	ActorType       domain.ActorType     `json:"actor_type,omitempty"`
	ActorID         string               `json:"actor_id,omitempty"`
	Reason          string               `json:"reason"`
}

type modelPresetEnablementPreviewRequest struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
}

func (a *API) activeModelsRevision(ctx context.Context) (*domain.ConfigRevision, error) {
	var active *domain.ConfigRevision
	err := a.Events.Store.View(ctx, func(reader port.Reader) error {
		revision, loadErr := reader.ActiveConfigRevision(domain.ConfigScopeModels)
		if errors.Is(loadErr, port.ErrNotFound) {
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		active = &revision
		return nil
	})
	return active, err
}

func (a *API) handlePreviewModelPresetEnablement(w http.ResponseWriter, r *http.Request) {
	preset, ok := a.modelPreset(strings.TrimSpace(r.PathValue("presetID")))
	if !ok {
		writeAPIError(w, apiError{status: http.StatusNotFound, code: "not_found", message: "model preset not found"})
		return
	}
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req modelPresetEnablementPreviewRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if req.SchemaVersion != 0 && req.SchemaVersion != domain.SchemaVersionV1 {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "unsupported schema_version"})
		return
	}
	activeRevision, err := a.activeModelsRevision(r.Context())
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_revision"))
		return
	}
	var active *domain.ModelsConfig
	if activeRevision != nil {
		active = activeRevision.Models
	}
	preview, err := preset.PreviewEnablement(active, strings.TrimSpace(req.Version))
	if err == nil && a.Events.Store != nil {
		a.Events.Store.View(r.Context(), func(reader port.Reader) error {
			if pressure, pressureErr := reader.ModelContextPressure(preset.Binding.ID); pressureErr == nil && preview.ContextSummary != nil {
				preview.ContextSummary.ObservedPressure = &pressure
			}
			return nil
		})
	}
	if err != nil {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: sanitizeValidationMessage(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": domain.SchemaVersionV1, "preview": preview})
}

func (a *API) handleCreateModelPresetEnableDraft(w http.ResponseWriter, r *http.Request) {
	preset, ok := a.modelPreset(strings.TrimSpace(r.PathValue("presetID")))
	if !ok {
		writeAPIError(w, apiError{status: http.StatusNotFound, code: "not_found", message: "model preset not found"})
		return
	}
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req modelPresetDraftRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if req.SchemaVersion == 0 {
		req.SchemaVersion = domain.SchemaVersionV1
	}
	if req.SchemaVersion != domain.SchemaVersionV1 || strings.TrimSpace(req.Reason) == "" {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "supported schema_version and reason are required"})
		return
	}
	active, err := a.activeModelsRevision(r.Context())
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_revision"))
		return
	}
	if active == nil {
		writeAPIError(w, apiError{status: http.StatusConflict, code: "conflict", message: "preset must first be installed disabled as the active MODELS revision"})
		return
	}
	if req.BasedOnRevision != active.Revision {
		writeAPIError(w, apiError{status: http.StatusConflict, code: "conflict", message: "based_on_revision is stale"})
		return
	}
	preview, err := preset.PreviewEnablement(active.Models, strings.TrimSpace(req.Version))
	if err == nil && a.Events.Store != nil {
		a.Events.Store.View(r.Context(), func(reader port.Reader) error {
			if pressure, pressureErr := reader.ModelContextPressure(preset.Binding.ID); pressureErr == nil && preview.ContextSummary != nil {
				preview.ContextSummary.ObservedPressure = &pressure
			}
			return nil
		})
	}
	if err != nil {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: sanitizeValidationMessage(err)})
		return
	}
	if preview.Blocked || preview.Candidate == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"schema_version": domain.SchemaVersionV1, "preview": preview, "error": errorDetail{Code: "conflict", Message: "preset enablement is blocked"}})
		return
	}
	actorType := req.ActorType
	if actorType == "" {
		actorType = a.ActorType
	}
	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = a.DefaultActorID
	}
	draftID := req.DraftID
	if draftID == "" {
		generated, idErr := a.IDs.NewID("cfgdraft")
		if idErr != nil {
			writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "identity_failed", message: "could not assign draft identity"})
			return
		}
		draftID = domain.ConfigDraftID(generated)
	}
	draft := domain.ConfigDraft{
		SchemaVersion: req.SchemaVersion, ID: draftID, Scope: domain.ConfigScopeModels,
		BasedOnRevision: active.Revision, Applicability: domain.ConfigRestartRequired,
		Status: domain.ConfigDraftOpen, ActorType: actorType, ActorID: actorID,
		Reason: strings.TrimSpace(req.Reason), Models: preview.Candidate, CreatedAt: a.Clock.Now().UTC(),
	}
	if err := draft.Validate(); err != nil {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: sanitizeValidationMessage(err)})
		return
	}
	if err := a.Events.Store.Update(r.Context(), func(tx port.Transaction) error { return tx.CreateConfigDraft(draft) }); err != nil {
		writeAPIError(w, mapStoreError(err, "config_draft"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"schema_version": domain.SchemaVersionV1, "preset_id": preset.ID, "draft": draft, "preview": preview, "accepted": true})
}

func (a *API) handleCreateModelPresetDraft(w http.ResponseWriter, r *http.Request) {
	preset, ok := a.modelPreset(strings.TrimSpace(r.PathValue("presetID")))
	if !ok {
		writeAPIError(w, apiError{status: http.StatusNotFound, code: "not_found", message: "model preset not found"})
		return
	}
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req modelPresetDraftRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if req.SchemaVersion == 0 {
		req.SchemaVersion = domain.SchemaVersionV1
	}
	active, err := a.activeModelsRevision(r.Context())
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_revision"))
		return
	}
	if active == nil {
		if req.BasedOnRevision != 0 {
			writeAPIError(w, apiError{status: http.StatusConflict, code: "conflict", message: "based_on_revision is stale"})
			return
		}
	} else if req.BasedOnRevision != active.Revision {
		writeAPIError(w, apiError{status: http.StatusConflict, code: "conflict", message: "based_on_revision is stale"})
		return
	}
	var activeModels *domain.ModelsConfig
	if active != nil {
		activeModels = active.Models
	}
	models, err := preset.ModelsConfigDraftFromActive(activeModels, strings.TrimSpace(req.Version))
	if err != nil {
		writeAPIError(w, apiError{status: http.StatusConflict, code: "conflict", message: sanitizeValidationMessage(err)})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "reason is required"})
		return
	}
	actorType := req.ActorType
	if actorType == "" {
		actorType = a.ActorType
	}
	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = a.DefaultActorID
	}
	draftID := req.DraftID
	if draftID == "" {
		generated, idErr := a.IDs.NewID("cfgdraft")
		if idErr != nil {
			writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "identity_failed", message: "could not assign draft identity"})
			return
		}
		draftID = domain.ConfigDraftID(generated)
	}
	draft := domain.ConfigDraft{
		SchemaVersion: req.SchemaVersion, ID: draftID, Scope: domain.ConfigScopeModels,
		BasedOnRevision: req.BasedOnRevision, Applicability: domain.DefaultApplicabilityForScope(domain.ConfigScopeModels),
		Status: domain.ConfigDraftOpen, ActorType: actorType, ActorID: actorID,
		Reason: reason, Models: &models, CreatedAt: a.Clock.Now().UTC(),
	}
	if err := draft.Validate(); err != nil {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: sanitizeValidationMessage(err)})
		return
	}
	if err := a.Events.Store.Update(r.Context(), func(tx port.Transaction) error { return tx.CreateConfigDraft(draft) }); err != nil {
		writeAPIError(w, mapStoreError(err, "config_draft"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"schema_version": domain.SchemaVersionV1, "preset_id": preset.ID,
		"draft": draft, "accepted": true,
	})
}

type missionAmendmentRequest struct {
	SchemaVersion        int                          `json:"schema_version"`
	MissionID            domain.MissionID             `json:"mission_id"`
	BaseRevision         uint64                       `json:"base_revision"`
	CandidateRevision    uint64                       `json:"candidate_revision"`
	OriginalText         string                       `json:"original_text"`
	Purpose              string                       `json:"purpose"`
	Domains              []string                     `json:"domains"`
	Policies             []string                     `json:"policies"`
	Budget               domain.Budget                `json:"budget"`
	Status               domain.MissionStatus         `json:"status"`
	StandingObjectives   []string                     `json:"standing_objectives,omitempty"`
	RecurringObligations []domain.RecurringObligation `json:"recurring_obligations,omitempty"`
	Reason               string                       `json:"reason"`
	// Provenance is accept-only; ignored on preview. Defaults to control actor.
	Provenance string `json:"provenance,omitempty"`
}

func (a *API) amendmentFromRequest(req missionAmendmentRequest) (domain.UserAmendment, error) {
	if req.SchemaVersion == 0 {
		req.SchemaVersion = domain.SchemaVersionV1
	}
	amendment := domain.UserAmendment{
		SchemaVersion:        req.SchemaVersion,
		MissionID:            req.MissionID,
		BaseRevision:         req.BaseRevision,
		CandidateRevision:    req.CandidateRevision,
		OriginalText:         req.OriginalText,
		Purpose:              req.Purpose,
		Domains:              append([]string(nil), req.Domains...),
		Policies:             append([]string(nil), req.Policies...),
		Budget:               req.Budget,
		Status:               req.Status,
		StandingObjectives:   append([]string(nil), req.StandingObjectives...),
		RecurringObligations: append([]domain.RecurringObligation(nil), req.RecurringObligations...),
		Reason:               req.Reason,
	}
	if err := amendment.Validate(); err != nil {
		return domain.UserAmendment{}, apiError{status: http.StatusBadRequest, code: "invalid_request", message: sanitizeValidationMessage(err)}
	}
	return amendment, nil
}

func (a *API) loadActiveMission(ctx context.Context, missionID domain.MissionID) (domain.MissionRevision, error) {
	if missionID == "" {
		return domain.MissionRevision{}, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "mission_id is required"}
	}
	if a.Events == nil || a.Events.Store == nil {
		return domain.MissionRevision{}, apiError{status: http.StatusServiceUnavailable, code: "not_configured", message: "mission store is not wired"}
	}
	var active domain.MissionRevision
	err := a.Events.Store.View(ctx, func(reader port.Reader) error {
		var err error
		active, err = reader.ActiveMissionRevision(missionID)
		return err
	})
	if err != nil {
		return domain.MissionRevision{}, mapStoreError(err, "mission")
	}
	return active, nil
}

func (a *API) handleGetActiveMission(w http.ResponseWriter, r *http.Request) {
	missionID := domain.MissionID(strings.TrimSpace(r.PathValue("missionID")))
	active, err := a.loadActiveMission(r.Context(), missionID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"mission":        active,
	})
}

func (a *API) purePreviewAmendment(ctx context.Context, amendment domain.UserAmendment) (domain.MissionRevision, domain.MissionRevision, domain.MissionDiff, domain.MissionImpactPreview, error) {
	active, err := a.loadActiveMission(ctx, amendment.MissionID)
	if err != nil {
		return domain.MissionRevision{}, domain.MissionRevision{}, domain.MissionDiff{}, domain.MissionImpactPreview{}, err
	}
	if amendment.BaseRevision != active.Revision {
		return domain.MissionRevision{}, domain.MissionRevision{}, domain.MissionDiff{}, domain.MissionImpactPreview{}, apiError{
			status:  http.StatusConflict,
			code:    "conflict",
			message: fmt.Sprintf("base_revision %d disagrees with active revision %d", amendment.BaseRevision, active.Revision),
		}
	}
	candidate, err := domain.CandidateFromAmendment(active, amendment)
	if err != nil {
		return domain.MissionRevision{}, domain.MissionRevision{}, domain.MissionDiff{}, domain.MissionImpactPreview{}, mapStoreError(err, "mission_amendment")
	}
	diff, err := domain.DiffMissionRevisions(active, candidate)
	if err != nil {
		return domain.MissionRevision{}, domain.MissionRevision{}, domain.MissionDiff{}, domain.MissionImpactPreview{}, mapStoreError(err, "mission_amendment")
	}
	impact, err := domain.PreviewMissionImpact(active, candidate, diff)
	if err != nil {
		return domain.MissionRevision{}, domain.MissionRevision{}, domain.MissionDiff{}, domain.MissionImpactPreview{}, mapStoreError(err, "mission_amendment")
	}
	return active, candidate, diff, impact, nil
}

func (a *API) handlePreviewMissionAmendment(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req missionAmendmentRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	amendment, err := a.amendmentFromRequest(req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	previous, candidate, diff, impact, err := a.purePreviewAmendment(r.Context(), amendment)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"previous":       previous,
		"candidate":      candidate,
		"diff":           diff,
		"impact":         impact,
		// accepted is always false for preview; no write occurred.
		"accepted": false,
	})
}

func (a *API) handleAcceptMissionAmendment(w http.ResponseWriter, r *http.Request) {
	if a.MissionAccept == nil {
		writeAPIError(w, apiError{status: http.StatusServiceUnavailable, code: "not_configured", message: "mission amendment acceptance is not wired"})
		return
	}
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req missionAmendmentRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	amendment, err := a.amendmentFromRequest(req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	// Fail closed on pure no-op/blocked before invoking the acceptor.
	_, _, _, impact, err := a.purePreviewAmendment(r.Context(), amendment)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if impact.Blocked || !impact.RequiresAcceptance {
		writeAPIError(w, apiError{
			status:  http.StatusConflict,
			code:    "conflict",
			message: "mission amendment is blocked or does not require acceptance",
		})
		return
	}
	provenance := strings.TrimSpace(req.Provenance)
	if provenance == "" {
		actor := strings.TrimSpace(a.DefaultActorID)
		if actor == "" {
			actor = "operator_local"
		}
		provenance = "user:" + actor
	}
	result, err := a.MissionAccept.Accept(r.Context(), amendment, provenance)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "mission_amendment"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"previous":       result.Previous,
		"accepted":       result.Accepted,
		"diff":           result.Diff,
		"impact":         result.Impact,
		"report":         result.Report,
		"installed":      true,
	})
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
	SchemaVersion      int                      `json:"schema_version"`
	EventID            domain.ExternalEventID   `json:"event_id"`
	DeduplicationKey   string                   `json:"deduplication_key"`
	Source             string                   `json:"source"`
	SourceActorID      string                   `json:"source_actor_id"`
	Kind               domain.ExternalEventKind `json:"kind"`
	MissionID          domain.MissionID         `json:"mission_id,omitempty"`
	CorrelationID      string                   `json:"correlation_id,omitempty"`
	TransportMessageID string                   `json:"transport_message_id,omitempty"`
	Content            domain.ExternalContent   `json:"content"`
	ReceivedAt         *time.Time               `json:"received_at,omitempty"`
}

type externalEventSubmitResponse struct {
	SchemaVersion int                             `json:"schema_version"`
	EventID       domain.ExternalEventID          `json:"event_id"`
	Disposition   domain.ExternalEventDisposition `json:"disposition"`
	// Accepted means the stimulus was durably received. Kernel disposition may
	// later mark it applied, rejected, or ignored.
	Accepted bool `json:"accepted"`
}

type questionAnswerSubmitRequest struct {
	SchemaVersion            int                       `json:"schema_version"`
	AnswerID                 domain.OperatorAnswerID   `json:"answer_id,omitempty"`
	IdempotencyKey           string                    `json:"idempotency_key"`
	ExpectedQuestionRevision uint64                    `json:"expected_question_revision"`
	Kind                     domain.OperatorAnswerKind `json:"kind"`
	OptionIDs                []string                  `json:"option_ids,omitempty"`
	Text                     string                    `json:"text,omitempty"`
	ActorID                  string                    `json:"actor_id,omitempty"`
	SubmittedAt              *time.Time                `json:"submitted_at,omitempty"`
}

type questionAnswerSubmitResponse struct {
	SchemaVersion int                             `json:"schema_version"`
	QuestionID    domain.OperatorQuestionID       `json:"question_id"`
	AnswerID      domain.OperatorAnswerID         `json:"answer_id"`
	EventID       domain.ExternalEventID          `json:"event_id"`
	Disposition   domain.ExternalEventDisposition `json:"disposition"`
	Accepted      bool                            `json:"accepted"`
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

type configDraftCreateRequest struct {
	SchemaVersion   int                               `json:"schema_version"`
	DraftID         domain.ConfigDraftID              `json:"draft_id,omitempty"`
	Scope           domain.ConfigScope                `json:"scope"`
	BasedOnRevision uint64                            `json:"based_on_revision"`
	Applicability   domain.ConfigApplicability        `json:"applicability"`
	ActorType       domain.ActorType                  `json:"actor_type"`
	ActorID         string                            `json:"actor_id"`
	Reason          string                            `json:"reason"`
	Runtime         *domain.RuntimeProcessConfig      `json:"runtime,omitempty"`
	Scheduler       *domain.SchedulerCadenceConfig    `json:"scheduler,omitempty"`
	Horizon         *domain.HorizonPolicy             `json:"horizon,omitempty"`
	Interruption    *domain.InterruptionRuntimePolicy `json:"interruption,omitempty"`
	Channels        *domain.ChannelsConfig            `json:"channels,omitempty"`
	Models          *domain.ModelsConfig              `json:"models,omitempty"`
	CreatedAt       *time.Time                        `json:"created_at,omitempty"`
}

func (a *API) handleCreateConfigDraft(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req configDraftCreateRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if req.SchemaVersion == 0 {
		req.SchemaVersion = domain.SchemaVersionV1
	}
	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = a.DefaultActorID
	}
	actorType := req.ActorType
	if actorType == "" {
		actorType = a.ActorType
	}
	applicability := req.Applicability
	if applicability == "" {
		applicability = domain.DefaultApplicabilityForScope(req.Scope)
	}
	createdAt := a.Clock.Now().UTC()
	if req.CreatedAt != nil {
		createdAt = req.CreatedAt.UTC()
	}
	draftID := req.DraftID
	if draftID == "" {
		id, idErr := a.IDs.NewID("cfgdraft")
		if idErr != nil {
			writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "identity_failed", message: "could not assign draft identity"})
			return
		}
		draftID = domain.ConfigDraftID(id)
	}
	draft := domain.ConfigDraft{
		SchemaVersion: req.SchemaVersion, ID: draftID, Scope: req.Scope,
		BasedOnRevision: req.BasedOnRevision, Applicability: applicability,
		Status: domain.ConfigDraftOpen, ActorType: actorType, ActorID: actorID,
		Reason: strings.TrimSpace(req.Reason), Runtime: req.Runtime, Scheduler: req.Scheduler,
		Horizon: req.Horizon, Interruption: req.Interruption, Channels: req.Channels, Models: req.Models,
		CreatedAt: createdAt,
	}
	if err := draft.Validate(); err != nil {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: sanitizeValidationMessage(err)})
		return
	}
	err = a.Events.Store.Update(r.Context(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(draft)
	})
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_draft"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"draft":          draft,
		"accepted":       true,
	})
}

func (a *API) handleListConfigDrafts(w http.ResponseWriter, r *http.Request) {
	scope := domain.ConfigScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	if !scope.Valid() {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "valid scope is required"})
		return
	}
	status := domain.ConfigDraftStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && !status.Valid() {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "unknown draft status"})
		return
	}
	var drafts []domain.ConfigDraft
	err := a.Events.Store.View(r.Context(), func(reader port.Reader) error {
		var err error
		drafts, err = reader.ConfigDrafts(scope, status)
		return err
	})
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_draft"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"scope":          scope,
		"status":         status,
		"drafts":         drafts,
	})
}

func (a *API) handleGetConfigDraft(w http.ResponseWriter, r *http.Request) {
	id := domain.ConfigDraftID(r.PathValue("draftID"))
	var draft domain.ConfigDraft
	var receipt domain.ConfigApplyReceipt
	hasReceipt := false
	err := a.Events.Store.View(r.Context(), func(reader port.Reader) error {
		var err error
		draft, err = reader.ConfigDraft(id)
		if err != nil {
			return err
		}
		if got, recErr := reader.ConfigApplyReceipt(id); recErr == nil {
			receipt = got
			hasReceipt = true
			return nil
		} else if !errors.Is(recErr, port.ErrNotFound) {
			return recErr
		}
		return nil
	})
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_draft"))
		return
	}
	payload := map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"draft":          draft,
	}
	if hasReceipt {
		payload["receipt"] = receipt
	}
	writeJSON(w, http.StatusOK, payload)
}

func (a *API) handleValidateConfigDraft(w http.ResponseWriter, r *http.Request) {
	if a.ConfigValidate == nil {
		writeAPIError(w, apiError{status: http.StatusServiceUnavailable, code: "not_configured", message: "config validation is not wired"})
		return
	}
	id := domain.ConfigDraftID(r.PathValue("draftID"))
	preview, diff, err := a.ConfigValidate.ValidateDraft(r.Context(), id)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_draft"))
		return
	}
	var draft domain.ConfigDraft
	_ = a.Events.Store.View(r.Context(), func(reader port.Reader) error {
		var loadErr error
		draft, loadErr = reader.ConfigDraft(id)
		return loadErr
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"draft":          draft,
		"preview":        preview,
		"diff":           diff,
	})
}

func (a *API) handleApplyConfigDraft(w http.ResponseWriter, r *http.Request) {
	if a.ConfigApply == nil {
		writeAPIError(w, apiError{status: http.StatusServiceUnavailable, code: "not_configured", message: "config apply is not wired"})
		return
	}
	id := domain.ConfigDraftID(r.PathValue("draftID"))
	revision, receipt, err := a.ConfigApply.ApplyDraft(r.Context(), id)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_draft"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"revision":       revision,
		"receipt":        receipt,
	})
}

func (a *API) handleGetConfigApplyReceipt(w http.ResponseWriter, r *http.Request) {
	id := domain.ConfigDraftID(r.PathValue("draftID"))
	var receipt domain.ConfigApplyReceipt
	err := a.Events.Store.View(r.Context(), func(reader port.Reader) error {
		var err error
		receipt, err = reader.ConfigApplyReceipt(id)
		return err
	})
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_apply_receipt"))
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func (a *API) handleGetActiveConfigRevision(w http.ResponseWriter, r *http.Request) {
	scope := domain.ConfigScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	if !scope.Valid() {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "valid scope is required"})
		return
	}
	var revision domain.ConfigRevision
	err := a.Events.Store.View(r.Context(), func(reader port.Reader) error {
		var err error
		revision, err = reader.ActiveConfigRevision(scope)
		return err
	})
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_revision"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"revision":       revision,
	})
}

func (a *API) handleListConfigRevisions(w http.ResponseWriter, r *http.Request) {
	scope := domain.ConfigScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	if !scope.Valid() {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "valid scope is required"})
		return
	}
	var revisions []domain.ConfigRevision
	err := a.Events.Store.View(r.Context(), func(reader port.Reader) error {
		var err error
		revisions, err = reader.ConfigRevisions(scope)
		return err
	})
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_revision"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"scope":          scope,
		"revisions":      revisions,
	})
}

type configRollbackRequest struct {
	SchemaVersion int                     `json:"schema_version"`
	Scope         domain.ConfigScope      `json:"scope"`
	TargetID      domain.ConfigRevisionID `json:"target_revision_id"`
	ActorType     domain.ActorType        `json:"actor_type,omitempty"`
	ActorID       string                  `json:"actor_id,omitempty"`
	Reason        string                  `json:"reason,omitempty"`
}

func (a *API) handleRollbackConfigRevision(w http.ResponseWriter, r *http.Request) {
	if a.ConfigRollback == nil {
		writeAPIError(w, apiError{status: http.StatusServiceUnavailable, code: "not_configured", message: "config rollback is not wired"})
		return
	}
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req configRollbackRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if req.SchemaVersion == 0 {
		req.SchemaVersion = domain.SchemaVersionV1
	}
	if req.SchemaVersion != domain.SchemaVersionV1 || !req.Scope.Valid() || req.TargetID == "" {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "scope and target_revision_id are required"})
		return
	}
	actorType := req.ActorType
	if actorType == "" {
		actorType = a.ActorType
	}
	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = a.DefaultActorID
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "semantic rollback via control API"
	}
	draft, revision, receipt, err := a.ConfigRollback.RollbackToRevision(r.Context(), req.Scope, req.TargetID, actorType, actorID, reason)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "config_revision"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"draft":          draft,
		"revision":       revision,
		"receipt":        receipt,
		"rolled_back_to": req.TargetID,
	})
}

func (a *API) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	missionID := domain.MissionID(strings.TrimSpace(r.URL.Query().Get("mission_id")))
	if missionID == "" {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "mission_id is required"})
		return
	}
	status := domain.OperatorQuestionStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = domain.OperatorQuestionPending
	}
	if !knownQuestionStatus(status) {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "unknown question status"})
		return
	}
	var questions []domain.OperatorQuestion
	err := a.Events.Store.View(r.Context(), func(reader port.Reader) error {
		var err error
		questions, err = reader.OperatorQuestions(missionID, status)
		return err
	})
	if err != nil {
		writeAPIError(w, mapStoreError(err, "question"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"questions":      questions,
	})
}

func (a *API) handleGetQuestion(w http.ResponseWriter, r *http.Request) {
	question, err := a.question(r.Context(), domain.OperatorQuestionID(r.PathValue("questionID")))
	if err != nil {
		writeAPIError(w, mapStoreError(err, "question"))
		return
	}
	writeJSON(w, http.StatusOK, question)
}

func (a *API) handleSubmitQuestionAnswer(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var req questionAnswerSubmitRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	questionID := domain.OperatorQuestionID(r.PathValue("questionID"))
	question, err := a.question(r.Context(), questionID)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "question"))
		return
	}
	if req.SchemaVersion == 0 {
		req.SchemaVersion = domain.SchemaVersionV1
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "idempotency_key is required"})
		return
	}
	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = a.DefaultActorID
	}
	receivedAt := a.Clock.Now().UTC()
	if req.SubmittedAt != nil {
		receivedAt = req.SubmittedAt.UTC()
	}
	if existing, lookupErr := a.Events.ExternalEventByDeduplicationKey(strings.TrimSpace(req.IdempotencyKey)); lookupErr == nil {
		existingAnswer, decodeErr := decodeDashboardAnswer(existing)
		if decodeErr != nil || existing.CorrelationID != string(question.ID) || existing.SourceActorID != actorID ||
			existingAnswer.ExpectedQuestionRevision != req.ExpectedQuestionRevision || existingAnswer.Kind != req.Kind ||
			!equalStrings(existingAnswer.OptionIDs, req.OptionIDs) || existingAnswer.Text != req.Text ||
			(req.AnswerID != "" && existingAnswer.ID != req.AnswerID) {
			writeAPIError(w, apiError{status: http.StatusConflict, code: "conflict", message: "idempotency_key was already used for a different answer"})
			return
		}
		disposition, dispositionErr := a.Events.SubmitExternalEvent(existing)
		if dispositionErr != nil {
			writeAPIError(w, mapStoreError(dispositionErr, "question_answer"))
			return
		}
		writeJSON(w, http.StatusAccepted, questionAnswerSubmitResponse{
			SchemaVersion: domain.SchemaVersionV1, QuestionID: question.ID, AnswerID: existingAnswer.ID,
			EventID: existing.ID, Disposition: disposition, Accepted: true,
		})
		return
	} else if lookupErr != nil && !errors.Is(lookupErr, port.ErrNotFound) {
		writeAPIError(w, mapStoreError(lookupErr, "question_answer"))
		return
	}
	answerID := req.AnswerID
	if answerID == "" {
		id, idErr := a.IDs.NewID("answer")
		if idErr != nil {
			writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "identity_failed", message: "could not assign answer identity"})
			return
		}
		answerID = domain.OperatorAnswerID(id)
	}
	answer := domain.UserAnswer{
		SchemaVersion:            req.SchemaVersion,
		ID:                       answerID,
		QuestionID:               questionID,
		ExpectedQuestionRevision: req.ExpectedQuestionRevision,
		Kind:                     req.Kind,
		OptionIDs:                append([]string(nil), req.OptionIDs...),
		Text:                     req.Text,
		ActorID:                  actorID,
		Channel:                  "operator-dashboard",
		TransportEventID:         strings.TrimSpace(req.IdempotencyKey),
		ReceivedAt:               receivedAt,
	}
	if err := answer.ValidateForQuestion(question); err != nil {
		writeAPIError(w, apiError{status: http.StatusConflict, code: "invalid_answer", message: sanitizeValidationMessage(err)})
		return
	}
	structured, err := json.Marshal(answer)
	if err != nil {
		writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "encode_failed", message: "could not encode answer"})
		return
	}
	event := domain.ExternalEvent{
		SchemaVersion:    domain.SchemaVersionV1,
		DeduplicationKey: answer.TransportEventID,
		Source:           answer.Channel,
		SourceActorID:    answer.ActorID,
		Kind:             domain.ExternalUserAnswer,
		MissionID:        question.MissionID,
		CorrelationID:    string(question.ID),
		Content: domain.ExternalContent{
			MediaType:  "application/json",
			Structured: structured,
		},
		ReceivedAt: receivedAt,
	}
	filled, err := ensureExternalEventIdentity(event, a.Clock, a.IDs)
	if err != nil {
		writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "identity_failed", message: "could not assign answer event identity"})
		return
	}
	disposition, err := a.Events.SubmitExternalEvent(filled)
	if err != nil {
		writeAPIError(w, mapStoreError(err, "question_answer"))
		return
	}
	writeJSON(w, http.StatusAccepted, questionAnswerSubmitResponse{
		SchemaVersion: domain.SchemaVersionV1,
		QuestionID:    question.ID,
		AnswerID:      answer.ID,
		EventID:       filled.ID,
		Disposition:   disposition,
		Accepted:      true,
	})
}

func (a *API) question(ctx context.Context, id domain.OperatorQuestionID) (domain.OperatorQuestion, error) {
	if id == "" {
		return domain.OperatorQuestion{}, port.ErrNotFound
	}
	var question domain.OperatorQuestion
	err := a.Events.Store.View(ctx, func(reader port.Reader) error {
		var err error
		question, err = reader.OperatorQuestion(id)
		return err
	})
	return question, err
}

func knownQuestionStatus(status domain.OperatorQuestionStatus) bool {
	switch status {
	case domain.OperatorQuestionPending, domain.OperatorQuestionClarificationRequested, domain.OperatorQuestionAnswered,
		domain.OperatorQuestionExpired, domain.OperatorQuestionSuperseded, domain.OperatorQuestionCancelled:
		return true
	default:
		return false
	}
}

func decodeDashboardAnswer(event domain.ExternalEvent) (domain.UserAnswer, error) {
	if event.Kind != domain.ExternalUserAnswer || event.Source != "operator-dashboard" {
		return domain.UserAnswer{}, errors.New("event is not a dashboard answer")
	}
	var answer domain.UserAnswer
	if err := decodeStrictJSON(event.Content.Structured, &answer); err != nil {
		return domain.UserAnswer{}, err
	}
	return answer, answer.Validate()
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
	case errors.Is(err, port.ErrConflict), errors.Is(err, domain.ErrConflict):
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
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "disagrees") ||
		strings.Contains(msg, "blocked") ||
		strings.Contains(msg, "does not require")
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

type memorySubmitRequest struct {
	ID         domain.MemoryID    `json:"id"`
	Key        string             `json:"key"`
	Scope      domain.MemoryScope `json:"scope"`
	Value      string             `json:"value"`
	Expiration time.Time          `json:"expiration,omitempty"`
}

func (a *API) handleSubmitMemory(w http.ResponseWriter, r *http.Request) {
	if a.SemanticMemory == nil {
		writeAPIError(w, apiError{status: http.StatusServiceUnavailable, code: "unavailable", message: "semantic memory unavailable"})
		return
	}
	var req memorySubmitRequest
	body, err := readLimitedJSON(r)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if err := decodeStrictJSON(body, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if req.ID == "" || strings.TrimSpace(req.Key) == "" || strings.TrimSpace(req.Value) == "" {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "id, key and value are required"})
		return
	}
	if req.Scope != domain.MemoryScopeMission && req.Scope != domain.MemoryScopeStrategy && req.Scope != domain.MemoryScopeAgent {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "invalid memory scope"})
		return
	}
	mem := domain.LongTermMemory{ID: req.ID, Key: req.Key, Scope: req.Scope, Value: req.Value, StoredAt: a.Clock.Now().UTC(), Expiration: req.Expiration}
	if err := a.SemanticMemory.SaveMemory(r.Context(), mem); err != nil {
		writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "internal_error", message: "save memory failed"})
		return
	}
	writeJSON(w, http.StatusCreated, mem)
}
func (a *API) handleListMemories(w http.ResponseWriter, r *http.Request) {
	if a.SemanticMemoryReader == nil {
		writeAPIError(w, apiError{status: http.StatusServiceUnavailable, code: "unavailable", message: "semantic memory unavailable"})
		return
	}

	scopeParam := r.URL.Query().Get("scope")

	var memories []domain.LongTermMemory
	var err error

	if scopeParam != "" {
		scope := domain.MemoryScope(scopeParam)
		if scope != domain.MemoryScopeMission && scope != domain.MemoryScopeStrategy && scope != domain.MemoryScopeAgent {
			writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "invalid memory scope"})
			return
		}
		memories, err = a.SemanticMemoryReader.ListMemoriesByScope(scope)
	} else {
		// As there is no list all method in the MemoryReader yet,
		// we fetch each scope explicitly and combine them if no scope is provided.
		mission, err1 := a.SemanticMemoryReader.ListMemoriesByScope(domain.MemoryScopeMission)
		strategy, err2 := a.SemanticMemoryReader.ListMemoriesByScope(domain.MemoryScopeStrategy)
		agent, err3 := a.SemanticMemoryReader.ListMemoriesByScope(domain.MemoryScopeAgent)

		if err1 == nil && err2 == nil && err3 == nil {
			memories = append(memories, mission...)
			memories = append(memories, strategy...)
			memories = append(memories, agent...)
		} else {
			err = errors.New("failed to read multiple scopes")
		}
	}

	if err != nil {
		writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "internal_error", message: "failed to list memories"})
		return
	}

	// Expiration removes a memory from the readable current view immediately;
	// physical compaction and its audit event may happen in a later bounded job.
	now := a.Clock.Now()
	active := memories[:0]
	for _, memory := range memories {
		if memory.Expiration.IsZero() || memory.Expiration.After(now) {
			active = append(active, memory)
		}
	}
	memories = active

	if memories == nil {
		memories = make([]domain.LongTermMemory, 0)
	}

	writeJSON(w, http.StatusOK, memories)
}

func (a *API) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if a.SemanticMemory == nil {
		writeAPIError(w, apiError{status: http.StatusServiceUnavailable, code: "unavailable", message: "semantic memory unavailable"})
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeAPIError(w, apiError{status: http.StatusBadRequest, code: "invalid_request", message: "missing memory ID"})
		return
	}

	deleted, err := a.SemanticMemory.DeleteMemory(r.Context(), domain.MemoryID(id), "operator_deleted")
	if err != nil {
		writeAPIError(w, apiError{status: http.StatusInternalServerError, code: "internal_error", message: "failed to delete memory"})
		return
	}
	if !deleted {
		writeAPIError(w, apiError{status: http.StatusNotFound, code: "not_found", message: "memory not found"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
