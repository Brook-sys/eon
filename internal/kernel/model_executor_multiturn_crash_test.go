package kernel

import (
	"context"
	"encoding/json"
	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
	"motor-autonomo/internal/tool"
	"path/filepath"
	"testing"
	"time"
)

type resumedMultiTurnProvider struct {
	calls int
}
func (p *resumedMultiTurnProvider) ID() string { return "resumed-fake" }
func (p *resumedMultiTurnProvider) Kind() domain.ProviderKind {
	return domain.ProviderKindOpenAICompatible
}
func (p *resumedMultiTurnProvider) Profile() domain.ProviderProfile {
	return domain.ProviderProfile{MaxOutputTokens: 1024, MaxContextTokens: 4096}
}
func (p *resumedMultiTurnProvider) Complete(ctx context.Context, req port.CompletionRequest) (port.CompletionResult, error) {
	return p.CompleteWithTools(ctx, req, nil)
}
func (p *resumedMultiTurnProvider) CompleteWithTools(ctx context.Context, req port.CompletionRequest, tools []port.ToolDefinition) (port.CompletionResult, error) {
	p.calls++
	return port.CompletionResult{
		Model: "resumed-fake",
		Text:  `{"changes":[{"kind":"ADD","entity_type":"observation","entity_id":"obs_final","payload_ref":"payload"}],"expected_delta":"one observation","validator_ids":["schema"]}`, 
	}, nil
}

type dummyMultiTool struct {
	def port.ToolDefinition
}
func (c *dummyMultiTool) Definition() port.ToolDefinition { return c.def }
func (c *dummyMultiTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "sunny", nil
}

func TestModelExecutorMultiTurnReplaysReceipt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "model_multiturn.sqlite")
	now := time.Date(2026, 7, 22, 19, 0, 0, 0, time.UTC).UTC()
	ctx := context.Background()

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	seedModelAgenda(t, store, now)

	leaseRef := FormatLeaseRef("lease_01", "operation_model", 1, now.Add(5*time.Minute))
	err = store.Update(ctx, func(tx port.Transaction) error {
		op, _ := tx.Operation("operation_model")
		op.State = domain.StateRunning
		op.Attempt = 1
		op.Reevaluation = domain.ReevaluationCondition{Kind: domain.ReevaluateLease, Reference: leaseRef}
		tx.SaveOperation(op)
		
		res := port.DurableModelCompletionResult(port.CompletionResult{
			Model: "fake",
			ToolCalls: []port.ToolCall{
				{ID: "tc_1", Name: "get_weather", Arguments: `{}`},
			},
		})
		h, _ := res.Hash()

		receipt := domain.ModelCompletionReceipt{
			SchemaVersion: domain.SchemaVersionV1,
			OperationID:   "operation_model",
			Attempt:       1,
			ModelCall:     1,
			RecordedAt:    now,
			Result:        res,
			PayloadHash:   h,
		}
		return tx.AppendModelCompletionReceipt(receipt)
	})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	store2, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	clock := source.NewManualClock(now.Add(10 * time.Minute))
	ids := source.NewSequenceIDGenerator(100)
	
	reaper := LeaseReaper{Store: store2, Clock: clock, IDs: ids}
	rec, err := reaper.Reconcile(ctx, "revision_1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Reconciled != 1 {
		t.Fatalf("expected 1 lease reconciled, got %d", rec.Reconciled)
	}

	prov := &resumedMultiTurnProvider{}
	
	weatherDef := port.ToolDefinition{
		Name:        "get_weather",
		Description: "Get weather",
		Parameters:  []byte(`{"type":"object","properties":{"location":{"type":"string"}}}`),
	}
	tools, _ := tool.NewCatalog(&dummyMultiTool{def: weatherDef})
	processor, _ := changeset.New(changeset.Config{Store: store2, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})

	exec := ModelExecutor{
		Store:               store2,
		Clock:               clock,
		PolicyVersion:       "policy@model-test",
		IDs:                 ids,
		PrimaryProviderID:   "p1",
		PrimaryBindingID:    "b1",
		PrimaryProviderKind: domain.ProviderKindOpenAICompatible,
		Provider:            prov,
		Providers: map[string]port.ModelProvider{
			"b1": prov,
		},
		Changes: processor,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 4096,
		},
		Tools: tools,
	}

	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatal(err)
	}

	if !result.Completed {
		t.Errorf("expected Completed=true")
	}
	
	if prov.calls != 1 {
		t.Errorf("expected provider to be called exactly 1 time in the resumed process, got %d", prov.calls)
	}
}
