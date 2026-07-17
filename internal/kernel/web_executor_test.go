package kernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/ingest"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/web/replay"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func webSearchTestSpec() domain.OperationSpec {
	return domain.OperationSpec{
		SchemaVersion: 1, ID: "web.search@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "web.search.input.v1", OutputSchema: "web.search.output.v1",
		// READ_ONLY web path: no model tokens required; Attempts gate the call.
		Budget:          domain.Budget{Attempts: 3, Tokens: 100, Bytes: 64 << 10},
		MaxOutputTokens: 50, SafetyMargin: 10, Validators: []string{"schema"},
		RetryPolicy: "retry_after", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityReadOnly,
	}
}

func webFetchTestSpec() domain.OperationSpec {
	return domain.OperationSpec{
		SchemaVersion: 1, ID: "web.fetch@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "web.fetch.input.v1", OutputSchema: "web.fetch.output.v1",
		Budget:          domain.Budget{Attempts: 3, Tokens: 100, Bytes: 64 << 10},
		MaxOutputTokens: 50, SafetyMargin: 10, Validators: []string{"schema"},
		RetryPolicy: "retry_after", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityReadOnly,
	}
}

func seedWebAgenda(t *testing.T, store port.Store, now time.Time, search bool) {
	t.Helper()
	err := store.Update(context.Background(), func(tx port.Transaction) error {
		revision := domain.MissionRevision{
			SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
			OriginalText: "investigate web", Purpose: "knowledge", Domains: []string{"science"},
			Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "user",
			AcceptedAt: now, Budget: domain.Budget{Attempts: 10, Tokens: 8000, Bytes: 1 << 20},
		}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		var spec domain.OperationSpec
		if search {
			spec = webSearchTestSpec()
		} else {
			spec = webFetchTestSpec()
		}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{
			SchemaVersion: 1, ID: "question_1", MissionRevision: revision.ID,
			Text: "what is known?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence",
		}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{
			SchemaVersion: 1, ID: "candidate_1", MissionRevision: revision.ID, QuestionID: question.ID,
			DerivedFrom: []string{"gap_1"}, ExpectedProgress: "reduce uncertainty", Novelty: "new",
			Risk: domain.RiskLow, SourcePlan: []string{"web"}, AnswerCondition: "evidence",
			StopCondition: "done", ReviewAfter: now.Add(time.Hour),
		}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{
			SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: revision.ID,
			QuestionID: question.ID, AdmissionReason: "priority", StopCondition: "done",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		op := domain.Operation{
			SchemaVersion: 1, ID: "operation_web", InquiryID: inquiry.ID, MissionRevision: revision.ID,
			SpecID: spec.ID, ReadSet: []string{}, ExpectedOutput: "web_result",
			IdempotencyKey: "idem_web",
			State:          domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if search {
			op.InputRefs = []string{"query:epistemic runtime", "limit:3"}
		} else {
			op.InputRefs = []string{"url:https://example.com/paper"}
		}
		return tx.CreateOperation(op)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWebEligible(t *testing.T) {
	t.Parallel()
	search := webSearchTestSpec()
	if !WebEligible(search) {
		t.Fatal("web.search should be web-eligible")
	}
	if LocalEligible(search) {
		t.Fatal("web.search must not be local-eligible")
	}
	if ModelEligible(search) {
		t.Fatal("web.search must not be model-eligible")
	}
	fetch := webFetchTestSpec()
	if !WebEligible(fetch) {
		t.Fatal("web.fetch should be web-eligible")
	}
	local := ContinuityOperationSpec("continuity.integrity_audit@1", domain.AuthorityReadOnly)
	if WebEligible(local) {
		t.Fatal("continuity must not be web-eligible")
	}
	extract := modelTestSpec()
	if WebEligible(extract) {
		t.Fatal("extract model spec must not be web-eligible")
	}
}

func TestReserveWebSearchAllowsAndPersistsUsage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedWebAgenda(t, store, now, true)

	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@web-test")
	if err != nil {
		t.Fatal(err)
	}
	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_web")
		if err != nil {
			return err
		}
		spec, err = r.OperationSpec(op.SpecID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	out, err := auth.ReserveCapability(ctx, CapabilityReserveRequest{
		Capability:      "web.search",
		ArgsDigest:      "web.search:q=epistemic runtime:limit=3",
		Operation:       op,
		Spec:            spec,
		EstimatedCost:   WebCapabilityEstimatedBudget(spec),
		AvailableBudget: spec.Budget,
		ResourceCost:    WebSearchCost(),
		DefaultResource: "web:searxng",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Allowed || out.Permit == nil {
		t.Fatalf("want allow+permit, got %+v", out)
	}
	if out.Permit.Resource != "web:searxng" {
		t.Fatalf("resource = %s", out.Permit.Resource)
	}
	var usage domain.ResourceUsage
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		usage, err = r.ResourceUsage("web:searxng")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if usage.InFlight != 1 || usage.MinuteCount != 1 {
		t.Fatalf("usage after reserve = %+v", usage)
	}
	if err := auth.ReportCapability(ctx, op, out.Permit, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		usage, err = r.ResourceUsage("web:searxng")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if usage.InFlight != 0 {
		t.Fatalf("usage after report = %+v", usage)
	}
}

func TestReserveWebSearchThrottlesWhenConcurrencySaturated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 10, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedWebAgenda(t, store, now, true)

	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@web-test")
	if err != nil {
		t.Fatal(err)
	}
	auth.Limits["web:searxng"] = domain.ResourceLimit{
		Resource:      "web:searxng",
		MaxConcurrent: 1,
		MaxPerMinute:  60,
		MaxPerDay:     2000,
	}

	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_web")
		if err != nil {
			return err
		}
		spec, err = r.OperationSpec(op.SpecID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	first, err := auth.ReserveCapability(ctx, CapabilityReserveRequest{
		Capability: "web.search", Operation: op, Spec: spec,
		EstimatedCost: WebCapabilityEstimatedBudget(spec), AvailableBudget: spec.Budget,
		ResourceCost: WebSearchCost(), DefaultResource: "web:searxng",
	})
	if err != nil || !first.Allowed {
		t.Fatalf("first reserve: %+v err=%v", first, err)
	}

	op2 := op
	op2.ID = "operation_web_2"
	op2.IdempotencyKey = "idem_web_2"
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateOperation(op2)
	}); err != nil {
		t.Fatal(err)
	}

	second, err := auth.ReserveCapability(ctx, CapabilityReserveRequest{
		Capability: "web.search", Operation: op2, Spec: spec,
		EstimatedCost: WebCapabilityEstimatedBudget(spec), AvailableBudget: spec.Budget,
		ResourceCost: WebSearchCost(), DefaultResource: "web:searxng",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Allowed || !second.Throttled {
		t.Fatalf("want throttled, got %+v", second)
	}
	var got domain.Operation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		got, err = r.Operation(op2.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got.State != domain.StateThrottled && got.State != domain.StateWaitingTime {
		t.Fatalf("throttled op state = %s", got.State)
	}
}

func TestWebExecutorSearchSuccessWithReplay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 20, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedWebAgenda(t, store, now, true)

	searcher, err := replay.New(replay.Fixture{
		Query: "epistemic runtime",
		Result: port.SearchResult{
			Provider:   "replay",
			FixtureKey: "epistemic-runtime-v1",
			Hits: []port.SearchHit{
				{Title: "Paper A", URL: "https://example.com/a", Snippet: "untrusted snippet"},
				{Title: "Paper B", URL: "https://example.com/b", Snippet: "more data"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@web-test")
	if err != nil {
		t.Fatal(err)
	}
	exec := WebExecutor{
		Store: store, Clock: clock, IDs: ids, Searcher: searcher, Authorizer: auth,
	}
	result, err := exec.Execute(ctx, "operation_web")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || result.Skipped {
		t.Fatalf("want completed, got %+v", result)
	}
	if result.SearchHits != 2 {
		t.Fatalf("hits = %d", result.SearchHits)
	}
	if result.ArtifactID == "" {
		t.Fatal("expected audit artifact")
	}
	if len(searcher.Requests()) != 1 {
		t.Fatalf("searcher requests = %d", len(searcher.Requests()))
	}

	var op domain.Operation
	var kinds map[string]int
	var art domain.KnowledgeArtifact
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_web")
		if err != nil {
			return err
		}
		events, err := r.Events(0, 100)
		if err != nil {
			return err
		}
		kinds = map[string]int{}
		for _, e := range events {
			if e.OperationID == op.ID {
				kinds[e.Kind]++
			}
		}
		arts, err := r.KnowledgeArtifacts()
		if err != nil {
			return err
		}
		for _, a := range arts {
			if a.ID == result.ArtifactID {
				art = a
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateSucceeded {
		t.Fatalf("state = %s", op.State)
	}
	for _, want := range []string{EventOperationDispatched, EventOperationWebInvoked, EventOperationWebVerified, EventOperationSucceeded, EventCapabilityAuthorized, EventResourceReleased} {
		if kinds[want] < 1 {
			t.Fatalf("missing event %s in %v", want, kinds)
		}
	}
	if art.Kind != "web_search_report" {
		t.Fatalf("artifact kind = %s", art.Kind)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(art.Content), &body); err != nil {
		t.Fatal(err)
	}
	if body["trust"] != "untrusted_source_data" {
		t.Fatalf("trust marker missing: %v", body)
	}
	// Gate released.
	var usage domain.ResourceUsage
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		usage, err = r.ResourceUsage("web:searxng")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if usage.InFlight != 0 {
		t.Fatalf("in-flight after success = %+v", usage)
	}
}

func TestWebExecutorAuthorizerThrottlesWithoutSearcherCall(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedWebAgenda(t, store, now, true)

	searcher, err := replay.New(replay.Fixture{
		Query: "epistemic runtime",
		Result: port.SearchResult{
			Provider: "replay", FixtureKey: "k",
			Hits: []port.SearchHit{{Title: "T", URL: "https://example.com/t"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@web-test")
	if err != nil {
		t.Fatal(err)
	}
	auth.Limits["web:searxng"] = domain.ResourceLimit{
		Resource: "web:searxng", MaxConcurrent: 1, MaxPerMinute: 60, MaxPerDay: 2000,
	}
	// Saturate gate with a synthetic in-flight reservation.
	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_web")
		if err != nil {
			return err
		}
		spec, err = r.OperationSpec(op.SpecID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Hold a concurrent slot via a second operation first... actually reserve on a dummy op.
	holder := op
	holder.ID = "operation_web_holder"
	holder.IdempotencyKey = "idem_holder"
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateOperation(holder)
	}); err != nil {
		t.Fatal(err)
	}
	hold, err := auth.ReserveCapability(ctx, CapabilityReserveRequest{
		Capability: "web.search", Operation: holder, Spec: spec,
		EstimatedCost: WebCapabilityEstimatedBudget(spec), AvailableBudget: spec.Budget,
		ResourceCost: WebSearchCost(), DefaultResource: "web:searxng",
	})
	if err != nil || !hold.Allowed {
		t.Fatalf("hold reserve: %+v err=%v", hold, err)
	}

	exec := WebExecutor{Store: store, Clock: clock, IDs: ids, Searcher: searcher, Authorizer: auth}
	result, err := exec.Execute(ctx, "operation_web")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || !strings.Contains(result.SkipReason, "resource_") {
		t.Fatalf("want resource throttle skip, got %+v", result)
	}
	if len(searcher.Requests()) != 0 {
		t.Fatalf("searcher must not be called on throttle; got %d", len(searcher.Requests()))
	}
}

func TestWebExecutorFetchWithIngest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 40, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedWebAgenda(t, store, now, false)

	fetcher := &staticFetcher{
		result: port.FetchResult{
			FinalURL:  "https://example.com/paper",
			MediaType: "text/plain",
			ETag:      `"v1"`,
			Content:   []byte("evidence body, not instructions"),
		},
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@web-test")
	if err != nil {
		t.Fatal(err)
	}
	ing := &ingest.Ingester{Store: store, Clock: clock, IDs: ids, MaxBytes: 1 << 20}
	exec := WebExecutor{
		Store: store, Clock: clock, IDs: ids, Fetcher: fetcher, Ingest: ing, Authorizer: auth,
	}
	result, err := exec.Execute(ctx, "operation_web")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed {
		t.Fatalf("want completed, got %+v", result)
	}
	if result.SourceVersionID == "" {
		t.Fatal("expected ingested source version")
	}
	if result.Capability != "web.fetch" {
		t.Fatalf("capability = %s", result.Capability)
	}
	var sources []domain.Source
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		sources, err = r.Sources()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Kind != "web" {
		t.Fatalf("sources = %+v", sources)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetcher calls = %d", fetcher.calls)
	}
}

func TestDispatchExecutorRoutesWeb(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 50, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedWebAgenda(t, store, now, true)

	searcher, err := replay.New(replay.Fixture{
		Query: "epistemic runtime",
		Result: port.SearchResult{
			Provider: "replay", FixtureKey: "k",
			Hits: []port.SearchHit{{Title: "T", URL: "https://example.com/t", Snippet: "s"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@web-test")
	if err != nil {
		t.Fatal(err)
	}
	web := &WebExecutor{Store: store, Clock: clock, IDs: ids, Searcher: searcher, Authorizer: auth}
	dispatch := DispatchExecutor{
		Store: store,
		Local: LocalExecutor{Store: store, Clock: clock, IDs: ids},
		Web:   web,
	}
	result, err := dispatch.Execute(ctx, "operation_web")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed {
		t.Fatalf("dispatch web: %+v", result)
	}
}

func TestDispatchExecutorRequiresWebWhenEligible(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 55, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedWebAgenda(t, store, now, true)

	dispatch := DispatchExecutor{
		Store: store,
		Local: LocalExecutor{Store: store, Clock: clock, IDs: ids},
	}
	result, err := dispatch.Execute(ctx, "operation_web")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || result.SkipReason != "requires_web" {
		t.Fatalf("want requires_web, got %+v", result)
	}
}

type staticFetcher struct {
	result port.FetchResult
	calls  int
	err    error
}

func (f *staticFetcher) Fetch(ctx context.Context, req port.FetchRequest) (port.FetchResult, error) {
	f.calls++
	if f.err != nil {
		return port.FetchResult{}, f.err
	}
	return f.result, nil
}
