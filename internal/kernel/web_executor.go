package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/ingest"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// Web path audit event kinds. Search/fetch content is untrusted source data only
// (FR-RES-002); never policy, never executable instructions.
const (
	EventOperationWebInvoked  = "operation.web_invoked"
	EventOperationWebVerified = "operation.web_verified"
)

// WebExecutor runs READ_ONLY web.search / web.fetch operations under a lease
// with FR-RES-001 PolicyEngine + ResourceGate enforcement when Authorizer is set.
// Adapters must treat responses as hostile acquisition: bounded bytes, timeouts,
// and exact fixture/replay identity for deterministic tests.
type WebExecutor struct {
	Store    port.Store
	Clock    source.Clock
	IDs      source.IDGenerator
	Searcher port.WebSearcher // required for web.search
	Fetcher  port.WebFetcher  // required for web.fetch
	// Ingest is optional; when set, successful web.fetch materializes Source lineage.
	Ingest *ingest.Ingester
	// Authorizer is optional FR-RES-001 enforcement (nil = legacy allow).
	Authorizer *CapabilityAuthorizer
	LeaseTTL   time.Duration
	// DefaultSearchLimit caps hits when the operation does not specify limit.
	DefaultSearchLimit int
}

// WebExecuteResult summarizes one web-backed Execute call.
type WebExecuteResult struct {
	OperationID domain.OperationID
	Completed   bool
	Skipped     bool
	SkipReason  string
	LeaseRef    string
	ArtifactID  domain.ArtifactID
	// Capability is web.search or web.fetch when resolved.
	Capability string
	// SourceVersionID is set when fetch ingestion produced a SourceVersion.
	SourceVersionID domain.SourceVersionID
	// SearchHits is the number of hits returned (search path).
	SearchHits int
}

func (e WebExecutor) validateDeps() error {
	if e.Store == nil || e.Clock == nil || e.IDs == nil {
		return errors.New("web executor dependencies are incomplete")
	}
	return nil
}

func (e WebExecutor) leaseTTL() time.Duration {
	if e.LeaseTTL <= 0 {
		return 5 * time.Minute
	}
	return e.LeaseTTL
}

func (e WebExecutor) searchLimit() int {
	if e.DefaultSearchLimit > 0 {
		return e.DefaultSearchLimit
	}
	return 5
}

// WebEligible reports whether an OperationSpec should run on the web path.
// Continuity catalogue stays on LocalExecutor; PROPOSE_ONLY model path stays separate.
// READ_ONLY web.search/web.fetch specs are web-eligible (not LocalEligible).
func WebEligible(spec domain.OperationSpec) bool {
	if err := spec.Validate(); err != nil {
		return false
	}
	id := string(spec.ID)
	if strings.HasPrefix(id, "continuity.") {
		return false
	}
	// PROPOSE_ONLY non-web contracts remain on ModelExecutor.
	if spec.MaximumAuthority == domain.AuthorityProposeOnly && webCapabilityFromSpec(spec) == "" {
		return false
	}
	capName := webCapabilityFromSpec(spec)
	return capName == "web.search" || capName == "web.fetch"
}

// webCapabilityFromSpec maps OperationSpec identity/schemas to a catalog capability.
// Spec ID prefixes: web.search*, web.fetch*; schemas web.search.* / web.fetch.*.
func webCapabilityFromSpec(spec domain.OperationSpec) string {
	id := strings.ToLower(string(spec.ID))
	in := strings.ToLower(spec.InputSchema)
	out := strings.ToLower(spec.OutputSchema)
	switch {
	case strings.HasPrefix(id, "web.search") || strings.Contains(in, "web.search") || strings.Contains(out, "web.search"):
		return "web.search"
	case strings.HasPrefix(id, "web.fetch") || strings.Contains(in, "web.fetch") || strings.Contains(out, "web.fetch"):
		return "web.fetch"
	default:
		return ""
	}
}

func (e WebExecutor) releaseResourcePermit(ctx context.Context, operation domain.Operation, permit *domain.ResourcePermit, success bool) {
	if e.Authorizer == nil || permit == nil {
		return
	}
	_ = e.Authorizer.ReportCapability(ctx, operation, permit, success, nil)
}

// Execute runs one READY, web-eligible operation.
func (e WebExecutor) Execute(ctx context.Context, operationID domain.OperationID) (WebExecuteResult, error) {
	if err := e.validateDeps(); err != nil {
		return WebExecuteResult{}, err
	}
	if operationID == "" {
		return WebExecuteResult{}, errors.New("operation id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var result WebExecuteResult
	result.OperationID = operationID

	var (
		operation domain.Operation
		spec      domain.OperationSpec
		leaseRef  string
		now       time.Time
		permit    *domain.ResourcePermit
		capName   string
	)

	// Phase 0: load READY web-eligible operation.
	err := e.Store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation(operationID)
		if err != nil {
			return err
		}
		if op.State.Terminal() {
			result.Skipped = true
			result.SkipReason = "terminal"
			return nil
		}
		if op.State != domain.StateReady {
			result.Skipped = true
			result.SkipReason = "not_ready"
			return nil
		}
		loadedSpec, err := r.OperationSpec(op.SpecID)
		if err != nil {
			return fmt.Errorf("load operation spec %s: %w", op.SpecID, err)
		}
		if !WebEligible(loadedSpec) {
			result.Skipped = true
			result.SkipReason = "not_web_eligible"
			return nil
		}
		operation = op
		spec = loadedSpec
		capName = webCapabilityFromSpec(loadedSpec)
		result.Capability = capName
		return nil
	})
	if err != nil {
		return result, err
	}
	if result.Skipped {
		return result, nil
	}

	// Resolve args before reserve so ArgsDigest binds the effect.
	args, argsErr := parseWebArgs(operation, capName)
	if argsErr != nil {
		// Invalid args are a known non-effect; leave READY with skip (no throttle).
		result.Skipped = true
		result.SkipReason = "invalid_web_args"
		return result, nil
	}

	// Phase 0b: FR-RES-001 authorize capability + ResourceGate before lease.
	if e.Authorizer != nil {
		reserve := CapabilityReserveRequest{
			Capability:      capName,
			ArgsDigest:      webArgsDigest(capName, args),
			Operation:       operation,
			Spec:            spec,
			EstimatedCost:   WebCapabilityEstimatedBudget(spec),
			AvailableBudget: spec.Budget,
			Priority:        0,
		}
		switch capName {
		case "web.search":
			reserve.ResourceCost = WebSearchCost()
			reserve.DefaultResource = "web:searxng"
		case "web.fetch":
			reserve.ResourceCost = WebFetchCost(0)
			reserve.DefaultResource = "web:http"
		}
		auth, authErr := e.Authorizer.ReserveCapability(ctx, reserve)
		if authErr != nil {
			return result, authErr
		}
		if auth.Throttled {
			result.Skipped = true
			result.SkipReason = auth.SkipReason
			if result.SkipReason == "" {
				result.SkipReason = "resource_throttled"
			}
			return result, nil
		}
		if !auth.Allowed {
			result.Skipped = true
			result.SkipReason = auth.SkipReason
			if result.SkipReason == "" {
				result.SkipReason = "policy_deny"
			}
			return result, nil
		}
		permit = auth.Permit
	}

	// Phase 1: claim lease READY → RUNNING.
	err = e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operationID)
		if err != nil {
			return err
		}
		if op.State.Terminal() {
			result.Skipped = true
			result.SkipReason = "terminal"
			return nil
		}
		if op.State != domain.StateReady {
			result.Skipped = true
			result.SkipReason = "not_ready"
			return nil
		}
		loadedSpec, err := tx.OperationSpec(op.SpecID)
		if err != nil {
			return fmt.Errorf("load operation spec %s: %w", op.SpecID, err)
		}
		if !WebEligible(loadedSpec) {
			result.Skipped = true
			result.SkipReason = "not_web_eligible"
			return nil
		}
		leaseID, err := e.IDs.NewID("lease")
		if err != nil {
			return fmt.Errorf("generate lease id: %w", err)
		}
		if strings.TrimSpace(leaseID) == "" {
			return errors.New("generated lease id must not be empty")
		}
		now = e.Clock.Now().UTC()
		until := now.Add(e.leaseTTL())
		leaseRef = FormatLeaseRef(leaseID, op.ID, op.Attempt+1, until)
		result.LeaseRef = leaseRef

		snap := domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation}
		running, err := domain.Transition(snap, domain.TransitionInput{Event: domain.EventDispatch, Reference: leaseRef})
		if err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		op.State = running.State
		op.Reevaluation = running.Reevaluation
		op.Attempt++
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:dispatched:%d", op.ID, op.Attempt)),
			Kind:            EventOperationDispatched,
			OccurredAt:      now,
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      leaseRef + ";capability=" + capName,
		}); err != nil {
			return err
		}
		operation = op
		spec = loadedSpec
		return nil
	})
	if err != nil {
		e.releaseResourcePermit(ctx, operation, permit, false)
		return result, err
	}
	if result.Skipped {
		e.releaseResourcePermit(ctx, operation, permit, false)
		return result, nil
	}

	// Phase 2: external effect outside the write transaction.
	var (
		auditBody   map[string]any
		sourceVerID domain.SourceVersionID
		hitCount    int
		effectOK    bool
		effectErr   error
	)
	switch capName {
	case "web.search":
		if e.Searcher == nil {
			effectErr = errors.New("web executor requires a WebSearcher for web.search")
			break
		}
		limit := args.Limit
		if limit < 1 {
			limit = e.searchLimit()
		}
		searchResult, err := e.Searcher.Search(ctx, port.SearchRequest{Query: args.Query, Limit: limit})
		if err != nil {
			effectErr = fmt.Errorf("web search: %w", err)
			break
		}
		hitCount = len(searchResult.Hits)
		result.SearchHits = hitCount
		hits := make([]map[string]string, 0, len(searchResult.Hits))
		for _, h := range searchResult.Hits {
			hits = append(hits, map[string]string{
				"title":   h.Title,
				"url":     h.URL,
				"snippet": h.Snippet,
			})
		}
		auditBody = map[string]any{
			"capability":  "web.search",
			"query":       args.Query,
			"limit":       limit,
			"provider":    searchResult.Provider,
			"fixture_key": searchResult.FixtureKey,
			"hits":        hits,
			// Explicit: search content is untrusted source data, never instructions.
			"trust": "untrusted_source_data",
		}
		effectOK = true
	case "web.fetch":
		if e.Fetcher == nil {
			effectErr = errors.New("web executor requires a WebFetcher for web.fetch")
			break
		}
		fetched, err := e.Fetcher.Fetch(ctx, port.FetchRequest{URL: args.URL})
		if err != nil {
			effectErr = fmt.Errorf("web fetch: %w", err)
			break
		}
		auditBody = map[string]any{
			"capability":    "web.fetch",
			"request_url":   args.URL,
			"final_url":     fetched.FinalURL,
			"media_type":    fetched.MediaType,
			"bytes":         len(fetched.Content),
			"etag":          fetched.ETag,
			"last_modified": fetched.LastModified,
			"trust":         "untrusted_source_data",
		}
		if e.Ingest != nil {
			ingested, err := e.Ingest.IngestFetched(ctx, operation.MissionRevision, fetched)
			if err != nil {
				effectErr = fmt.Errorf("ingest fetched: %w", err)
				break
			}
			sourceVerID = ingested.Version.ID
			result.SourceVersionID = sourceVerID
			auditBody["source_id"] = string(ingested.Source.ID)
			auditBody["source_version_id"] = string(ingested.Version.ID)
			auditBody["content_hash"] = ingested.Version.ContentHash
		} else {
			// Without ingester: still succeed with bounded metadata only (no body in store).
			auditBody["content_ingested"] = false
		}
		effectOK = true
	default:
		effectErr = fmt.Errorf("unsupported web capability %q", capName)
	}

	if !effectOK {
		failErr := e.failRunning(ctx, operation, leaseRef, effectErr)
		e.releaseResourcePermit(ctx, operation, permit, false)
		return result, failErr
	}

	// Phase 3: VERIFYING → audit artifact → SUCCEEDED.
	err = e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operationID)
		if err != nil {
			return err
		}
		if op.State != domain.StateRunning || op.Reevaluation.Reference != leaseRef {
			return fmt.Errorf("%w: operation lease changed during web effect", port.ErrConflict)
		}
		verifying, err := domain.Transition(
			domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
			domain.TransitionInput{Event: domain.EventBeginVerify, Reference: leaseRef},
		)
		if err != nil {
			return fmt.Errorf("begin verify: %w", err)
		}
		op.State = verifying.State
		op.Reevaluation = verifying.Reevaluation

		now = e.Clock.Now().UTC()
		artifact, err := e.buildWebArtifact(tx, op, spec, leaseRef, capName, auditBody, now)
		if err != nil {
			return err
		}
		if artifact.ID != "" {
			if err := tx.AppendKnowledgeArtifact(artifact); err != nil {
				return fmt.Errorf("append web artifact: %w", err)
			}
			result.ArtifactID = artifact.ID
		}

		done, err := domain.Transition(
			domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
			domain.TransitionInput{Event: domain.EventSucceed},
		)
		if err != nil {
			return fmt.Errorf("succeed: %w", err)
		}
		op.State = done.State
		op.Reevaluation = done.Reevaluation
		if err := tx.SaveOperation(op); err != nil {
			return err
		}

		payload := leaseRef + ";capability=" + capName
		if sourceVerID != "" {
			payload += ";source_version=" + string(sourceVerID)
		}
		if hitCount > 0 {
			payload += fmt.Sprintf(";hits=%d", hitCount)
		}
		for _, event := range []domain.Event{
			{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID(fmt.Sprintf("%s:web_invoked:%d", op.ID, op.Attempt)),
				Kind:          EventOperationWebInvoked,
				OccurredAt:    now, MissionRevision: op.MissionRevision, InquiryID: op.InquiryID,
				OperationID: op.ID, PayloadRef: payload,
			},
			{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID(fmt.Sprintf("%s:web_verified:%d", op.ID, op.Attempt)),
				Kind:          EventOperationWebVerified,
				OccurredAt:    now, MissionRevision: op.MissionRevision, InquiryID: op.InquiryID,
				OperationID: op.ID, PayloadRef: payload,
			},
			{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID(fmt.Sprintf("%s:succeeded:%d", op.ID, op.Attempt)),
				Kind:          EventOperationSucceeded,
				OccurredAt:    now, MissionRevision: op.MissionRevision, InquiryID: op.InquiryID,
				OperationID: op.ID, PayloadRef: payload,
			},
		} {
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		result.Completed = true
		return nil
	})
	if err != nil {
		e.releaseResourcePermit(ctx, operation, permit, false)
		return result, err
	}
	e.releaseResourcePermit(ctx, operation, permit, true)
	return result, nil
}

type webArgs struct {
	Query string
	Limit int
	URL   string
}

func parseWebArgs(operation domain.Operation, capability string) (webArgs, error) {
	var out webArgs
	// InputRefs convention:
	//   query:<text> | q:<text>
	//   limit:<n>
	//   url:<absolute-url> | https://... (literal)
	for _, ref := range operation.InputRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		lower := strings.ToLower(ref)
		switch {
		case strings.HasPrefix(lower, "query:"):
			out.Query = strings.TrimSpace(ref[len("query:"):])
		case strings.HasPrefix(lower, "q:"):
			out.Query = strings.TrimSpace(ref[len("q:"):])
		case strings.HasPrefix(lower, "limit:"):
			var n int
			if _, err := fmt.Sscanf(ref[len("limit:"):], "%d", &n); err == nil && n > 0 {
				out.Limit = n
			}
		case strings.HasPrefix(lower, "url:"):
			out.URL = strings.TrimSpace(ref[len("url:"):])
		case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
			out.URL = ref
		}
	}
	// ExpectedOutput may carry a compact query for search when refs omit it.
	if out.Query == "" && capability == "web.search" {
		eo := strings.TrimSpace(operation.ExpectedOutput)
		if strings.HasPrefix(strings.ToLower(eo), "query:") {
			out.Query = strings.TrimSpace(eo[len("query:"):])
		}
	}
	switch capability {
	case "web.search":
		if strings.TrimSpace(out.Query) == "" {
			return out, errors.New("web.search requires query input")
		}
	case "web.fetch":
		if strings.TrimSpace(out.URL) == "" {
			return out, errors.New("web.fetch requires url input")
		}
		u, err := url.Parse(out.URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return out, fmt.Errorf("web.fetch url is not absolute: %q", out.URL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return out, fmt.Errorf("web.fetch scheme %q not allowed", u.Scheme)
		}
	}
	return out, nil
}

func webArgsDigest(capability string, args webArgs) string {
	switch capability {
	case "web.search":
		return fmt.Sprintf("web.search:q=%s:limit=%d", args.Query, args.Limit)
	case "web.fetch":
		return "web.fetch:url=" + args.URL
	default:
		return capability
	}
}

func (e WebExecutor) buildWebArtifact(
	tx port.Transaction,
	operation domain.Operation,
	spec domain.OperationSpec,
	leaseRef string,
	capability string,
	body map[string]any,
	now time.Time,
) (domain.KnowledgeArtifact, error) {
	baseCommit := domain.GenesisCommitID
	if head, err := tx.HeadCommit(operation.MissionRevision); err == nil {
		baseCommit = head.ID
	}
	id, err := e.IDs.NewID("artifact")
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("generate artifact id: %w", err)
	}
	if body == nil {
		body = map[string]any{}
	}
	body["operation_id"] = string(operation.ID)
	body["spec_id"] = string(spec.ID)
	body["lease_ref"] = leaseRef
	body["capability"] = capability
	body["produced_at"] = now.UTC().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(body)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	kind := "web_search_report"
	if capability == "web.fetch" {
		kind = "web_fetch_report"
	}
	deps := []string{
		"operation:" + string(operation.ID),
		"spec:" + string(spec.ID),
		"lease:" + leaseRef,
	}
	art := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.ArtifactID(id),
		Kind:          kind,
		BaseCommitID:  baseCommit,
		Dependencies:  deps,
		ContentRef:    "inline:" + kind,
		Content:       string(encoded),
	}
	if err := art.Validate(); err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	return art, nil
}

func (e WebExecutor) failRunning(ctx context.Context, operation domain.Operation, leaseRef string, cause error) error {
	failErr := e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operation.ID)
		if err != nil {
			return err
		}
		if op.State != domain.StateRunning || op.Reevaluation.Reference != leaseRef {
			return nil
		}
		next, err := domain.Transition(
			domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
			domain.TransitionInput{Event: domain.EventRequestReplan, Reference: leaseRef},
		)
		if err != nil {
			return err
		}
		ready, err := domain.Transition(next, domain.TransitionInput{Event: domain.EventResume})
		if err != nil {
			return err
		}
		op.State = ready.State
		op.Reevaluation = ready.Reevaluation
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:web_fail:%d:%d", op.ID, op.Attempt, e.Clock.Now().UnixNano())),
			Kind:            "operation.web_failed",
			OccurredAt:      e.Clock.Now().UTC(),
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      leaseRef + ";error_class=web_or_ingest",
		})
		return err
	})
	if failErr != nil {
		return fmt.Errorf("%v; also failed to replan operation: %w", cause, failErr)
	}
	return cause
}
