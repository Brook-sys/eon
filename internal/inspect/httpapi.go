package inspect

import (
	"encoding/json"
	"errors"
	"fmt"
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
	mux.HandleFunc("GET /commits", a.handleCommits)
	mux.HandleFunc("GET /commits/{commitID}", a.handleCommit)
	mux.HandleFunc("GET /provider/profile", a.handleProviderProfile)
	mux.HandleFunc("GET /provider/profile/probe", a.handleProviderProfileProbe)
	mux.HandleFunc("GET /commands/{commandID}", a.handleCommand)
	mux.HandleFunc("GET /events", a.handleEvents)
	mux.HandleFunc("GET /events/{eventID}", a.handleEvent)
	mux.HandleFunc("GET /events/stream", a.handleEventStream)
	mux.HandleFunc("GET /knowledge", a.handleKnowledgeCatalog)
	mux.HandleFunc("GET /knowledge/sources", a.handleKnowledgeSources)
	mux.HandleFunc("GET /knowledge/sources/{sourceID}", a.handleKnowledgeSource)
	mux.HandleFunc("GET /knowledge/observations", a.handleKnowledgeObservations)
	mux.HandleFunc("GET /knowledge/observations/{observationID}", a.handleKnowledgeObservation)
	mux.HandleFunc("GET /knowledge/claims", a.handleKnowledgeClaims)
	mux.HandleFunc("GET /knowledge/claims/{claimID}", a.handleKnowledgeClaim)
	mux.HandleFunc("GET /knowledge/artifacts", a.handleKnowledgeArtifacts)
	mux.HandleFunc("GET /knowledge/artifacts/{artifactID}", a.handleKnowledgeArtifact)
	mux.HandleFunc("GET /continuity/findings", a.handleContinuityFindings)
	mux.HandleFunc("GET /continuity/catalog", a.handleContinuityCatalog)
	mux.HandleFunc("GET /frontier", a.handleFrontier)
	mux.HandleFunc("GET /frontier/hygiene", a.handleFrontierHygiene)
	mux.HandleFunc("GET /frontier/opportunities/{opportunityID}", a.handleFrontierOpportunity)
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
	payload := map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"runtime":        a.Projector.Runtime,
		"generated_at":   a.Projector.Clock().UTC().Format(time.RFC3339Nano),
	}
	if cat, ok := a.Projector.ContinuityCatalog(); ok {
		payload["continuity_catalog_version"] = cat.CatalogVersion
		payload["continuity_strategy_count"] = cat.StrategyCount
	}
	writeJSON(w, http.StatusOK, payload)
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
	safe, report := RedactOperationDetail(detail)
	writeJSON(w, http.StatusOK, OperationDetailResponse{
		OperationDetail: safe,
		Redaction:       report,
	})
}

func (a *API) handleCommits(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parseKnowledgePage(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	headOnly := false
	switch strings.TrimSpace(q.Get("head_only")) {
	case "", "0", "false", "no":
	case "1", "true", "yes":
		headOnly = true
	default:
		writeError(w, http.StatusBadRequest, "invalid_head_only", "head_only must be a boolean")
		return
	}
	page, err := a.Projector.ListCommits(r.Context(), limit, offset, CommitFilter{
		MissionRevision: domain.MissionRevisionID(strings.TrimSpace(q.Get("mission_revision_id"))),
		HeadOnly:        headOnly,
	})
	if err != nil {
		if strings.Contains(err.Error(), "limit must be") || strings.Contains(err.Error(), "offset must be") {
			writeError(w, http.StatusBadRequest, "invalid_page", err.Error())
			return
		}
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *API) handleCommit(w http.ResponseWriter, r *http.Request) {
	commitID := domain.CommitID(r.PathValue("commitID"))
	detail, err := a.Projector.CommitInspector(r.Context(), commitID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	safe, report := RedactCommitDetail(detail)
	writeJSON(w, http.StatusOK, CommitDetailResponse{
		CommitDetail: safe,
		Redaction:    report,
	})
}

func (a *API) handleProviderProfile(w http.ResponseWriter, r *http.Request) {
	view, err := a.Projector.ProviderProfile(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *API) handleProviderProfileProbe(w http.ResponseWriter, r *http.Request) {
	view, err := a.Projector.ProviderProfileProbe(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
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

func (a *API) handleKnowledgeCatalog(w http.ResponseWriter, r *http.Request) {
	summary, err := a.Projector.KnowledgeCatalog(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (a *API) handleKnowledgeSources(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parseKnowledgePage(w, r)
	if !ok {
		return
	}
	filter := KnowledgeSourceFilter{
		Kind: strings.TrimSpace(r.URL.Query().Get("kind")),
		Q:    strings.TrimSpace(r.URL.Query().Get("q")),
	}
	page, err := a.Projector.ListSources(r.Context(), limit, offset, filter)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *API) handleKnowledgeSource(w http.ResponseWriter, r *http.Request) {
	sourceID := domain.SourceID(r.PathValue("sourceID"))
	detail, err := a.Projector.SourceInspector(r.Context(), sourceID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *API) handleKnowledgeObservations(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parseKnowledgePage(w, r)
	if !ok {
		return
	}
	filter := KnowledgeObservationFilter{
		Provenance: strings.TrimSpace(r.URL.Query().Get("provenance")),
		Q:          strings.TrimSpace(r.URL.Query().Get("q")),
	}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("linked_only"))) {
	case "1", "true", "yes":
		filter.LinkedOnly = true
	case "", "0", "false", "no":
	default:
		writeError(w, http.StatusBadRequest, "invalid_filter", "linked_only must be true or false")
		return
	}
	page, err := a.Projector.ListObservations(r.Context(), limit, offset, filter)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	// Presentation redaction on list rows (statement/quote free text).
	for i := range page.Items {
		if page.Items[i].Statement != "" {
			text, _ := RedactSensitiveText(page.Items[i].Statement)
			page.Items[i].Statement, _ = BoundUTF8(text, knowledgeTextMax)
		}
		if page.Items[i].ExactQuote != "" {
			text, _ := RedactSensitiveText(page.Items[i].ExactQuote)
			page.Items[i].ExactQuote, _ = BoundUTF8(text, knowledgeTextMax)
		}
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *API) handleKnowledgeObservation(w http.ResponseWriter, r *http.Request) {
	observationID := domain.ObservationID(r.PathValue("observationID"))
	detail, err := a.Projector.ObservationInspector(r.Context(), observationID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	safe, report := RedactObservationDetail(detail)
	writeJSON(w, http.StatusOK, ObservationDetailResponse{
		ObservationDetail: safe,
		Redaction:         report,
	})
}

func (a *API) handleKnowledgeClaims(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parseKnowledgePage(w, r)
	if !ok {
		return
	}
	filter := KnowledgeClaimFilter{
		Q: strings.TrimSpace(r.URL.Query().Get("q")),
	}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("without_evidence"))) {
	case "1", "true", "yes":
		filter.WithoutEvidenceOnly = true
	case "", "0", "false", "no":
	default:
		writeError(w, http.StatusBadRequest, "invalid_filter", "without_evidence must be true or false")
		return
	}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("has_contradiction"))) {
	case "1", "true", "yes":
		filter.HasContradiction = true
	case "", "0", "false", "no":
	default:
		writeError(w, http.StatusBadRequest, "invalid_filter", "has_contradiction must be true or false")
		return
	}
	page, err := a.Projector.ListClaims(r.Context(), limit, offset, filter)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	for i := range page.Items {
		if page.Items[i].Proposition != "" {
			text, _ := RedactSensitiveText(page.Items[i].Proposition)
			page.Items[i].Proposition, _ = BoundUTF8(text, knowledgeTextMax)
		}
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *API) handleKnowledgeClaim(w http.ResponseWriter, r *http.Request) {
	claimID := domain.ClaimID(r.PathValue("claimID"))
	detail, err := a.Projector.ClaimInspector(r.Context(), claimID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	safe, report := RedactClaimDetail(detail)
	writeJSON(w, http.StatusOK, ClaimDetailResponse{
		ClaimDetail: safe,
		Redaction:   report,
	})
}

func (a *API) handleKnowledgeArtifacts(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parseKnowledgePage(w, r)
	if !ok {
		return
	}
	filter := KnowledgeArtifactFilter{
		Kind: strings.TrimSpace(r.URL.Query().Get("kind")),
		Q:    strings.TrimSpace(r.URL.Query().Get("q")),
	}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("stale"))) {
	case "1", "true", "yes":
		filter.StaleOnly = true
	case "", "0", "false", "no":
	default:
		writeError(w, http.StatusBadRequest, "invalid_filter", "stale must be true or false")
		return
	}
	page, err := a.Projector.ListArtifacts(r.Context(), limit, offset, filter)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *API) handleKnowledgeArtifact(w http.ResponseWriter, r *http.Request) {
	artifactID := domain.ArtifactID(r.PathValue("artifactID"))
	detail, err := a.Projector.ArtifactInspector(r.Context(), artifactID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	safe, report := RedactArtifactDetail(detail)
	writeJSON(w, http.StatusOK, ArtifactDetailResponse{
		ArtifactDetail: safe,
		Redaction:      report,
	})
}

// handleFrontier lists work opportunities for a mission's active revision.
// Query: mission_id (required), status, family, limit, offset.
func (a *API) handleFrontier(w http.ResponseWriter, r *http.Request) {
	missionID := domain.MissionID(strings.TrimSpace(r.URL.Query().Get("mission_id")))
	if missionID == "" {
		writeError(w, http.StatusBadRequest, "missing_mission_id", "mission_id is required")
		return
	}
	q := r.URL.Query()
	status, err := parseWorkOpportunityStatus(q.Get("status"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_status", err.Error())
		return
	}
	family, err := parseWorkFamily(q.Get("family"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_family", err.Error())
		return
	}
	limit, err := parseIntDefault(q.Get("limit"), DefaultFrontierListLimit)
	if err != nil || limit <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
		return
	}
	offset, err := parseIntDefault(q.Get("offset"), 0)
	if err != nil || offset < 0 {
		writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative integer")
		return
	}
	page, err := a.Projector.ListFrontier(r.Context(), missionID, FrontierListFilter{
		Status: status,
		Family: family,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		if strings.Contains(err.Error(), "limit must be") || strings.Contains(err.Error(), "offset must be") {
			writeError(w, http.StatusBadRequest, "invalid_page", err.Error())
			return
		}
		if strings.Contains(err.Error(), "unknown work") {
			writeError(w, http.StatusBadRequest, "invalid_filter", err.Error())
			return
		}
		writeStoreError(w, err)
		return
	}
	// Presentation redaction of free-text rows.
	for i := range page.Items {
		page.Items[i], _ = redactOpportunitySummary(page.Items[i])
	}
	writeJSON(w, http.StatusOK, page)
}

// handleFrontierHygiene dry-runs PlanFrontierReservoirHygiene without mutation.
// Query: mission_id (required).
func (a *API) handleFrontierHygiene(w http.ResponseWriter, r *http.Request) {
	missionID := domain.MissionID(strings.TrimSpace(r.URL.Query().Get("mission_id")))
	if missionID == "" {
		writeError(w, http.StatusBadRequest, "missing_mission_id", "mission_id is required")
		return
	}
	proj, err := a.Projector.FrontierHygieneForMission(r.Context(), missionID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

// handleFrontierOpportunity inspects one opportunity by id.
func (a *API) handleFrontierOpportunity(w http.ResponseWriter, r *http.Request) {
	opportunityID := domain.WorkOpportunityID(r.PathValue("opportunityID"))
	detail, err := a.Projector.OpportunityInspector(r.Context(), opportunityID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// handleContinuityCatalog returns the process-local versioned strategy portfolio.
// It does not require a mission_id: the catalogue is assembly metadata, not store state.
// When the projector has no catalogue configured, returns 404.
func (a *API) handleContinuityCatalog(w http.ResponseWriter, r *http.Request) {
	cat, ok := a.Projector.ContinuityCatalog()
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "continuity catalog not configured")
		return
	}
	writeJSON(w, http.StatusOK, cat)
}

// handleContinuityFindings projects model-free local continuity audit findings.
// Query:
//   - mission_id (required) scopes to the mission's active revision
//   - active_only=true drops stale KnowledgeArtifact reports
//   - family=<name> keeps only one continuity family (or report kind)
//
// Without mission_id, returns 400 — operators should always scope findings to a
// mission to avoid cross-mission leakage of free text.
func (a *API) handleContinuityFindings(w http.ResponseWriter, r *http.Request) {
	missionID := domain.MissionID(strings.TrimSpace(r.URL.Query().Get("mission_id")))
	if missionID == "" {
		writeError(w, http.StatusBadRequest, "missing_mission_id", "mission_id is required")
		return
	}
	q := r.URL.Query()
	filter := ContinuityFindingsFilter{
		Family: strings.TrimSpace(q.Get("family")),
	}
	switch strings.ToLower(strings.TrimSpace(q.Get("active_only"))) {
	case "1", "true", "yes", "on":
		filter.ActiveOnly = true
	case "", "0", "false", "no", "off":
		// default: include stale reports (still preferred lower in Latest ranking)
	default:
		writeError(w, http.StatusBadRequest, "invalid_active_only", "active_only must be a boolean")
		return
	}
	proj, err := a.Projector.ContinuityFindingsForMissionFiltered(r.Context(), missionID, filter)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

func parseKnowledgePage(w http.ResponseWriter, r *http.Request) (limit, offset int, ok bool) {
	q := r.URL.Query()
	var err error
	limit, err = parseIntDefault(q.Get("limit"), DefaultKnowledgeListLimit)
	if err != nil || limit <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
		return 0, 0, false
	}
	if limit > MaxKnowledgeListLimit {
		writeError(w, http.StatusBadRequest, "invalid_limit", fmt.Sprintf("limit must be between 1 and %d", MaxKnowledgeListLimit))
		return 0, 0, false
	}
	offset, err = parseIntDefault(q.Get("offset"), 0)
	if err != nil || offset < 0 {
		writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative integer")
		return 0, 0, false
	}
	return limit, offset, true
}

func writeKnowledgeError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "limit must be") || strings.Contains(err.Error(), "offset must be") {
		writeError(w, http.StatusBadRequest, "invalid_page", err.Error())
		return
	}
	writeStoreError(w, err)
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
