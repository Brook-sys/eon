package bootstrap

import (
	"bytes"
	"errors"
	"io"
	"log"
	"strings"

	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/tool"
)

type unresolvableToolProvider struct{}

func (unresolvableToolProvider) Definitions() []port.ToolDefinition {
	return []port.ToolDefinition{{Name: "missing_tool"}}
}

func (unresolvableToolProvider) Find(string) (tool.Tool, bool) { return nil, false }

func TestRuntimeReloadModelExecutorFailurePreservesCurrentExecutorAndRedactsCredentialReference(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := source.NewManualClock(time.Now())
	ids := source.NewSequenceIDGenerator(1)
	const credentialReference = "SENSITIVE_ACCOUNT_CREDENTIAL_REFERENCE"
	sentinel := errors.New("credential vault locked")

	current, err := BuildModelExecutor(Options{Model: &ModelOptions{
		Enabled: true, PolicyVersion: "v1", BaseURL: "http://localhost", Model: "current-model",
	}}, store, clock, ids, nil)
	if err != nil {
		t.Fatalf("build current executor: %v", err)
	}
	var logs bytes.Buffer
	rt := &Runtime{
		Opts:  Options{ModelSecretResolver: failingSecretResolver{err: sentinel}},
		Store: store, Clock: clock, IDs: ids, Model: current,
		logger: log.New(&logs, "", 0),
	}
	rt.Executor.Model = current

	seedModelsConfig(t, ctx, store, credentialReference, "reload-v2")
	err = rt.reloadModelExecutorIfNeeded(ctx)
	if !errors.Is(err, sentinel) {
		t.Fatalf("reload error = %v, want wrapped sentinel", err)
	}
	if rt.Model != current || rt.Executor.Model != current {
		t.Fatal("failed reload replaced the current executor")
	}
	if strings.Contains(err.Error(), credentialReference) || strings.Contains(logs.String(), credentialReference) {
		t.Fatalf("credential reference leaked: error=%q logs=%q", err, logs.String())
	}
}

func TestRuntimeReloadToolMergeFailurePreservesCurrentExecutor(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := source.NewManualClock(time.Now())
	ids := source.NewSequenceIDGenerator(1)
	current, err := BuildModelExecutor(Options{Model: &ModelOptions{
		Enabled: true, PolicyVersion: "v1", BaseURL: "http://localhost", Model: "current-model",
	}}, store, clock, ids, nil)
	if err != nil {
		t.Fatalf("build current executor: %v", err)
	}
	var logs bytes.Buffer
	rt := &Runtime{
		Opts: Options{}, Store: store, Clock: clock, IDs: ids, Model: current,
		logger: log.New(&logs, "", 0), subagentTools: unresolvableToolProvider{},
	}
	rt.Executor.Model = current
	t.Setenv("MERGE_FAILURE_TEST_KEY", "test-secret")
	seedModelsConfig(t, ctx, store, "MERGE_FAILURE_TEST_KEY", "merge-failure-v2")

	err = rt.reloadModelExecutorIfNeeded(ctx)
	if err == nil || !strings.Contains(err.Error(), "tool provider definition cannot be resolved") {
		t.Fatalf("reload error = %v, want tool merge failure", err)
	}
	if rt.Model != current || rt.Executor.Model != current {
		t.Fatal("failed tool merge replaced the current executor")
	}
	if !strings.Contains(logs.String(), "model executor tool merge failed") ||
		!strings.Contains(logs.String(), "tool provider definition cannot be resolved") {
		t.Fatalf("merge failure was not logged with its stable class: %q", logs.String())
	}
	if strings.Contains(logs.String(), "MERGE_FAILURE_TEST_KEY") || strings.Contains(logs.String(), "test-secret") {
		t.Fatalf("merge failure log leaked credential metadata: %q", logs.String())
	}
}

func seedModelsConfig(t *testing.T, ctx context.Context, store port.Store, credentialReference, version string) {
	t.Helper()
	err := store.Update(ctx, func(tx port.Transaction) error {
		limit := domain.ResourceLimit{Resource: "test", MaxConcurrent: 1, MaxPerMinute: 1, FailureThreshold: 1, CooldownBase: time.Second, CooldownMax: time.Second}
		models := &domain.ModelsConfig{Version: version,
			Providers: []domain.ModelProviderConfig{{ID: "p1", Kind: "openai_compatible", BaseURL: "http://localhost", Timeout: time.Second, MaxResponseBytes: 100, APIKeyEnv: credentialReference, GlobalLimit: limit}},
			Bindings:  []domain.ModelBindingConfig{{ID: "binding-1", ProviderRef: "p1", ModelID: "test-model", Enabled: true, Priority: 1, ContextTokens: 1000, MaxOutputTokens: 100, MaxOutputDialect: "max_tokens", Limit: limit}},
		}
		hash, _ := domain.ConfigPayloadHash(domain.ConfigScopeModels, nil, nil, nil, nil, nil, models)
		now := time.Now()
		draft := domain.ConfigDraft{SchemaVersion: 1, ID: domain.ConfigDraftID("draft_" + version), Scope: domain.ConfigScopeModels, Applicability: domain.ConfigHot, ActorType: domain.ActorOperator, ActorID: "test", Reason: "test", Status: domain.ConfigDraftOpen, Models: models, CreatedAt: now}
		if err := tx.CreateConfigDraft(draft); err != nil {
			return err
		}
		draft.Status, draft.ValidatedAt = domain.ConfigDraftValidated, now
		if err := tx.SaveConfigDraft(draft); err != nil {
			return err
		}
		draft.Status = domain.ConfigDraftApplied
		if err := tx.SaveConfigDraft(draft); err != nil {
			return err
		}
		rev := domain.ConfigRevision{SchemaVersion: 1, ID: domain.ConfigRevisionID("rev-" + version), Scope: domain.ConfigScopeModels, DraftID: draft.ID, Revision: 1, Applicability: domain.ConfigHot, ActorType: domain.ActorOperator, ActorID: "test", Reason: "test", AcceptedAt: now, Models: models, ContentHash: hash}
		if err := tx.AppendConfigRevision(rev); err != nil {
			return err
		}
		return tx.ActivateConfigRevision(domain.ConfigScopeModels, rev.ID)
	})
	if err != nil {
		t.Fatalf("seed models config: %v", err)
	}
}

func TestRuntimeReloadModelExecutorIfNeeded(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := source.NewManualClock(time.Now())
	ids := source.NewSequenceIDGenerator(1)

	opts := Options{
		Model: &ModelOptions{
			Enabled:       true,
			PolicyVersion: "v1",
			BaseURL:       "http://localhost",
			Model:         "test-model",
		},
	}

	rt := &Runtime{
		Opts:   opts,
		Store:  store,
		Clock:  clock,
		IDs:    ids,
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
			ID:            "draft_1",
			Scope:         domain.ConfigScopeModels,
			Applicability: domain.ConfigHot,
			ActorType:     domain.ActorOperator,
			ActorID:       "test",
			Reason:        "test",
			Status:        domain.ConfigDraftOpen,
			Models:        models,
			CreatedAt:     time.Now(),
		}
		if err := tx.CreateConfigDraft(draft); err != nil {
			return err
		}
		draft.Status = domain.ConfigDraftValidated
		draft.ValidatedAt = time.Now()
		if err := tx.SaveConfigDraft(draft); err != nil {
			return err
		}
		draft.Status = domain.ConfigDraftApplied
		if err := tx.SaveConfigDraft(draft); err != nil {
			return err
		}

		rev := domain.ConfigRevision{
			SchemaVersion: 1,
			ID:            "rev-1",
			Scope:         domain.ConfigScopeModels,
			DraftID:       "draft_1",
			Revision:      1,
			Applicability: domain.ConfigHot,
			ActorType:     domain.ActorOperator,
			ActorID:       "test",
			Reason:        "test",
			AcceptedAt:    time.Now(),
			Models:        models,
			ContentHash:   hash,
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
