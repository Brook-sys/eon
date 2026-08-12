package kernel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestModelExecutorExecutesReasoningOptions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	spec := modelTestSpec()
	
	seedModelAgendaWithSpec(t, store, now, spec)

	modelsConfig := domain.ModelsConfig{
		Version: "models@reasoning-test",
		Providers: []domain.ModelProviderConfig{
			{ID: "groq", Kind: domain.ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "GROQ_API_KEY", Timeout: 30 * time.Second, MaxResponseBytes: 1 << 20, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("groq")}},
		},
		Bindings: []domain.ModelBindingConfig{
			{ID: "primary", ProviderRef: "groq", ModelID: "test-model", Enabled: true, Priority: 10, ContextTokens: 32768, MaxOutputTokens: 1024, MaxOutputDialect: domain.MaxOutputDialectCompletion, ReasoningEffort: "low", ReasoningFormat: "hidden", Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("primary")}},
		},
	}
	_ = store.SetActiveConfig(context.Background(), domain.ConfigScopeModels, modelsConfig.Version, &modelsConfig)

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_1", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes:       []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_1", PayloadRef: "payload_1"}},
		ExpectedDelta: "test", ValidatorIDs: []string{"schema"},
		Provenance: "model:test", IdempotencyKey: "idem_model",
	}
	body, _ := json.Marshal(proposal)

	provider := &captureProvider{body: string(body)}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@reasoning-test"})
	if err != nil {
		t.Fatal(err)
	}

	exec := ModelExecutor{
		Store:         store,
		Clock:         clock,
		IDs:           ids,
		ModelsConfig:  &modelsConfig,
		Providers:     map[string]port.ModelProvider{"primary": provider},
		PrimaryBindingID: "primary",
		PrimaryProviderID: "groq",
		LeaseTTL:      5 * time.Minute,
		Changes:       processor,
		PolicyVersion: "policy@reasoning-test",
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 8000,
		},
	}

	res, err := exec.Execute(context.Background(), domain.OperationID("operation_model"))
	if err != nil {
		t.Fatal(err)
	}

	if !res.Completed && !res.Done && res.Skipped == false {
		t.Fatalf("expected some completion indication")
	}
	if provider.lastReq.ReasoningEffort != "low" {
		t.Fatalf("expected ReasoningEffort 'low', got %q", provider.lastReq.ReasoningEffort)
	}
	if provider.lastReq.ReasoningFormat != "hidden" {
		t.Fatalf("expected ReasoningFormat 'hidden', got %q", provider.lastReq.ReasoningFormat)
	}
}

type captureProvider struct {
	lastReq port.CompletionRequest
	body    string
}
func (c *captureProvider) Kind() domain.ProviderKind { return domain.ProviderKindGroq }
func (c *captureProvider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	c.lastReq = request
	return port.CompletionResult{Text: c.body, FinishReason: port.CompletionFinishStop, InputTokens: 10, OutputTokens: 5}, nil
}
func (c *captureProvider) Models(ctx context.Context) ([]string, error) { return nil, nil }
func (c *captureProvider) Close() error { return nil }
