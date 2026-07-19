package bootstrap

import (
	"io"
	"log"

	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestRuntimeReloadModelExecutorIfNeeded(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := source.NewManualClock(time.Now())
	ids := source.NewSequenceIDGenerator(1)
	
	opts := Options{
		Model: &ModelOptions{
			Enabled: true,
			PolicyVersion: "v1",
			BaseURL: "http://localhost",
			Model: "test-model",
		},
	}
	
	rt := &Runtime{
		Opts: opts,
		Store: store,
		Clock: clock,
		IDs: ids,
		logger: log.New(io.Discard, "", 0),
	}

	modelExec, err := BuildModelExecutor(opts, store, clock, ids, nil)
	if err != nil {
		t.Fatalf("failed to build model executor: %v", err)
	}
	rt.Model = modelExec
	rt.Executor.Model = modelExec

	if rt.Model.ModelsConfig != nil {
		t.Errorf("expected no active models config initially")
	}

	err = store.Update(ctx, func(tx port.Transaction) error {
		limit := domain.ResourceLimit{Resource: "test", MaxConcurrent: 1, MaxPerMinute: 1, FailureThreshold: 1, CooldownBase: time.Second, CooldownMax: time.Second}
		models := &domain.ModelsConfig{
			Version: "v2-reloaded",
			Providers: []domain.ModelProviderConfig{
				{ID: "p1", Kind: "openai_compatible", BaseURL: "http://localhost", Timeout: time.Second, MaxResponseBytes: 100, APIKeyEnv: "TEST_KEY", GlobalLimit: limit},
			},
			Bindings: []domain.ModelBindingConfig{
				{ID: "binding-1", ProviderRef: "p1", ModelID: "test-model", Enabled: true, Priority: 1, ContextTokens: 1000, MaxOutputTokens: 100, MaxOutputDialect: "max_tokens", Limit: limit},
			},
		}
		hash, _ := domain.ConfigPayloadHash(domain.ConfigScopeModels, nil, nil, nil, nil, nil, models)
		
		draft := domain.ConfigDraft{
		    SchemaVersion: 1,
		    ID: "draft_1",
		    Scope: domain.ConfigScopeModels,
		    Applicability: domain.ConfigHot,
		    ActorType: domain.ActorOperator,
		    ActorID: "test",
		    Reason: "test",
		    Status: domain.ConfigDraftOpen,
		    Models: models,
		    CreatedAt: time.Now(),
		}
		if err := tx.CreateConfigDraft(draft); err != nil { return err }
		draft.Status = domain.ConfigDraftValidated
		draft.ValidatedAt = time.Now()
		if err := tx.SaveConfigDraft(draft); err != nil { return err }
		draft.Status = domain.ConfigDraftApplied
		if err := tx.SaveConfigDraft(draft); err != nil { return err }

		rev := domain.ConfigRevision{
			SchemaVersion: 1,
			ID: "rev-1",
			Scope: domain.ConfigScopeModels,
			DraftID: "draft_1",
			Revision: 1,
			Applicability: domain.ConfigHot,
			ActorType: domain.ActorOperator,
			ActorID: "test",
			Reason: "test",
			AcceptedAt: time.Now(),
			Models: models,
			ContentHash: hash,
		}
		if err := tx.AppendConfigRevision(rev); err != nil {
			return err
		}
		return tx.ActivateConfigRevision(domain.ConfigScopeModels, rev.ID)
	})
	if err != nil {
		t.Fatalf("failed to inject models config: %v", err)
	}

	if err := rt.reloadModelExecutorIfNeeded(ctx); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	if rt.Model.ModelsConfig == nil {
		t.Fatalf("expected models config to be loaded")
	}
	if rt.Model.ModelsConfig.Version != "v2-reloaded" {
		t.Errorf("got %q, want v2-reloaded", rt.Model.ModelsConfig.Version)
	}
}
