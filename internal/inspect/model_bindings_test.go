package inspect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestListModelBindingPosturesCorrelatesOnlyPersistedEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 23, 25, 0, 0, time.UTC)
	store := memory.New()
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "test", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	empty, err := projector.ListModelBindingPostures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Count != 0 || empty.Note == "" || empty.Bindings == nil {
		t.Fatalf("empty = %+v", empty)
	}

	models := domain.ModelsConfig{
		Version: "models.v1",
		Providers: []domain.ModelProviderConfig{{
			ID: "groq", Kind: domain.ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1",
			APIKeyEnv: "GROQ_API_KEY", Timeout: time.Minute, MaxResponseBytes: 1 << 20,
			GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("groq"), MaxConcurrent: 2, MaxPerMinute: 10},
		}},
		Bindings: []domain.ModelBindingConfig{
			{ID: "disabled", ProviderRef: "groq", ModelID: "model-disabled", Enabled: false, Priority: 20, ContextTokens: 4096, MaxOutputTokens: 256, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("disabled"), MaxConcurrent: 1}},
			{ID: "primary", ProviderRef: "groq", ModelID: "model-primary", Enabled: true, Priority: 10, ContextTokens: 8192, MaxOutputTokens: 512, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("primary"), MaxConcurrent: 1, MaxTokensPerMinute: 2000}},
		},
	}
	if err := seedActiveModels(ctx, store, models, now); err != nil {
		t.Fatal(err)
	}
	openUntil := now.Add(time.Minute)
	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.SaveResourceUsage(domain.ResourceUsage{Resource: domain.ModelProviderResource("groq"), MinuteCount: 3}); err != nil {
			return err
		}
		if err := tx.SaveResourceUsage(domain.ResourceUsage{Resource: domain.ModelBindingResource("primary"), TokenMinuteCount: 400, CircuitOpenUntil: &openUntil}); err != nil {
			return err
		}
		return tx.SaveModelContextPressure(domain.ModelContextPressure{BindingID: "primary", State: domain.ContextPressureState{Level: 2, SuccessesAtLevel: 1}, UpdatedAt: now})
	}); err != nil {
		t.Fatal(err)
	}

	proj, err := projector.ListModelBindingPostures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if proj.ConfigRevision != "cfg_models_1" || proj.ConfigGeneration != 1 || proj.Count != 2 {
		t.Fatalf("proj = %+v", proj)
	}
	primary := proj.Bindings[0]
	if primary.BindingID != "primary" || primary.ProviderKind != domain.ProviderKindGroq || primary.ProviderUsage == nil || primary.BindingUsage == nil || primary.ContextPressure == nil {
		t.Fatalf("primary = %+v", primary)
	}
	if !primary.BindingUsage.CircuitOpen || primary.ContextPressure.ReductionFraction != 0.5 {
		t.Fatalf("evidence = %+v", primary)
	}
	disabled := proj.Bindings[1]
	if disabled.BindingID != "disabled" || disabled.Enabled || disabled.BindingUsage != nil || disabled.ContextPressure != nil {
		t.Fatalf("disabled = %+v", disabled)
	}
}

func TestModelBindingsHTTPEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 23, 30, 0, 0, time.UTC)
	store := memory.New()
	models := domain.ModelsConfig{Version: "models.v1", Providers: []domain.ModelProviderConfig{{ID: "nim", Kind: domain.ProviderKindNVIDIANIM, BaseURL: "https://integrate.api.nvidia.com/v1", APIKeyEnv: "NVIDIA_NIM_API_KEY", Timeout: time.Minute, MaxResponseBytes: 1 << 20, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("nim"), MaxConcurrent: 1}}}, Bindings: []domain.ModelBindingConfig{{ID: "nim-small", ProviderRef: "nim", ModelID: "mistralai/model", Enabled: true, Priority: 5, ContextTokens: 32768, MaxOutputTokens: 512, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("nim-small"), MaxConcurrent: 1}}}}
	if err := seedActiveModels(ctx, store, models, now); err != nil {
		t.Fatal(err)
	}
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "test", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/model-bindings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body inspect.ModelBindingsProjection
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 || body.Bindings[0].BindingID != "nim-small" || body.Bindings[0].ProviderUsage != nil {
		t.Fatalf("body = %+v", body)
	}
}

func seedActiveModels(ctx context.Context, store port.Store, models domain.ModelsConfig, now time.Time) error {
	hash, err := domain.ConfigPayloadHash(domain.ConfigScopeModels, nil, nil, nil, nil, nil, &models)
	if err != nil {
		return err
	}
	return store.Update(ctx, func(tx port.Transaction) error {
		draft := domain.ConfigDraft{SchemaVersion: domain.SchemaVersionV1, ID: "draft_models_1", Scope: domain.ConfigScopeModels, Applicability: domain.ConfigRestartRequired, Status: domain.ConfigDraftOpen, ActorType: domain.ActorOperator, ActorID: "fixture", Reason: "inspect model posture", Models: &models, CreatedAt: now}
		if err := tx.CreateConfigDraft(draft); err != nil {
			return err
		}
		rev := domain.ConfigRevision{SchemaVersion: domain.SchemaVersionV1, ID: "cfg_models_1", Scope: domain.ConfigScopeModels, Revision: 1, Applicability: domain.ConfigRestartRequired, ContentHash: hash, ActorType: domain.ActorOperator, ActorID: "fixture", Reason: "inspect model posture", DraftID: draft.ID, Models: &models, AcceptedAt: now}
		if err := tx.AppendConfigRevision(rev); err != nil {
			return err
		}
		return tx.ActivateConfigRevision(domain.ConfigScopeModels, rev.ID)
	})
}
