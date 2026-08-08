package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	memstore "motor-autonomo/internal/storage/memory"
)

func main() {
	ctx := context.Background()

	store := memstore.New()

	clock := source.SystemClock{}
	ids := source.CryptoIDGenerator{}

	// Step 1: Create a MODELS config draft
	draftID := domain.ConfigDraftID("cfgdraft_test1")
	modelsConfig := domain.ModelsConfig{
		Version: "models.v1",
		Providers: []domain.ModelProviderConfig{
			{
				ID:               "groq",
				Kind:             domain.ProviderKindGroq,
				BaseURL:          "https://api.groq.com/openai/v1",
				APIKeyEnv:        "GROQ_API_KEY",
				Timeout:          90 * time.Second,
				MaxResponseBytes: 10 * 1024 * 1024,
				GlobalLimit: domain.ResourceLimit{
					Resource:      "model-provider:groq",
					MaxConcurrent: 2,
					CooldownBase:  30 * time.Second,
					CooldownMax:   5 * time.Minute,
				},
			},
		},
		Bindings: []domain.ModelBindingConfig{
			{
				ID:               "binding_groq_llama70b",
				ProviderRef:      "groq",
				ModelID:          "llama-3.3-70b-versatile",
				Enabled:          true,
				Priority:         10,
				ContextTokens:    131072,
				MaxOutputTokens:  8192,
				MaxOutputDialect: domain.MaxOutputDialectLegacy,
				Limit: domain.ResourceLimit{
					Resource:      "model-binding:binding_groq_llama70b",
					MaxConcurrent: 1,
					CooldownBase:  30 * time.Second,
					CooldownMax:   5 * time.Minute,
				},
			},
		},
	}

	draft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            draftID,
		Scope:         domain.ConfigScopeModels,
		Applicability: domain.DefaultApplicabilityForScope(domain.ConfigScopeModels),
		Status:        domain.ConfigDraftOpen,
		ActorType:     domain.ActorOperator,
		ActorID:       "test-operator",
		Reason:        "test models flow",
		Models:        &modelsConfig,
		CreatedAt:     clock.Now().UTC(),
	}

	fmt.Printf("Step 1: Draft validation...\n")
	if err := draft.Validate(); err != nil {
		fmt.Printf("FAIL: draft validate: %v\n", err)
		os.Exit(1)
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateConfigDraft(draft)
	}); err != nil {
		fmt.Printf("FAIL: create draft: %v\n", err)
		os.Exit(1)
	}

	applier, err := kernel.NewConfigApplier(store, clock, ids)
	if err != nil {
		fmt.Printf("FAIL: create applier: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Step 2: Validate draft...\n")
	preview, diff, err := applier.ValidateDraft(ctx, draftID)
	if err != nil {
		fmt.Printf("FAIL: validate draft: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Preview: blocked=%v restartRequired=%v nextCycleOnly=%v\n", preview.Blocked, preview.RestartRequired, preview.NextCycleOnly)
	fmt.Printf("  Diff: empty=%v changes=%d\n", diff.Empty, len(diff.Changes))
	for _, c := range diff.Changes {
		fmt.Printf("    %s: %s -> %s\n", c.Path, c.Before, c.After)
	}

	fmt.Printf("Step 3: Apply draft...\n")
	revision, receipt, err := applier.ApplyDraft(ctx, draftID)
	if err != nil {
		fmt.Printf("FAIL: apply draft: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Revision: %s rev=%d\n", revision.ID, revision.Revision)
	fmt.Printf("  Receipt: state=%s\n", receipt.State)

	fmt.Printf("Step 4: Read active models config...\n")
	models, found, err := kernel.ActiveModelsConfig(ctx, store)
	if err != nil {
		fmt.Printf("FAIL: active models config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Found: %v\n", found)
	if found {
		fmt.Printf("  Version: %s\n", models.Version)
		fmt.Printf("  Providers: %d\n", len(models.Providers))
		for _, p := range models.Providers {
			fmt.Printf("    %s: kind=%s base_url=%s\n", p.ID, p.Kind, p.BaseURL)
		}
		fmt.Printf("  Bindings: %d\n", len(models.Bindings))
		for _, b := range models.Bindings {
			fmt.Printf("    %s: provider=%s model=%s enabled=%v dialect=%s\n", b.ID, b.ProviderRef, b.ModelID, b.Enabled, b.MaxOutputDialect)
		}
	}

	fmt.Printf("\nStep 5: Test with wrong dialect 'legacy' (as frontend sends)...\n")
	wrongBinding := modelsConfig
	wrongBinding.Bindings[0].MaxOutputDialect = "legacy"
	wrongDraft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "cfgdraft_test2",
		Scope:         domain.ConfigScopeModels,
		Applicability: domain.DefaultApplicabilityForScope(domain.ConfigScopeModels),
		Status:        domain.ConfigDraftOpen,
		ActorType:     domain.ActorOperator,
		ActorID:       "test-operator",
		Reason:        "test wrong dialect",
		Models:        &wrongBinding,
		CreatedAt:     clock.Now().UTC(),
	}
	if err := wrongDraft.Validate(); err != nil {
		fmt.Printf("  EXPECTED FAIL: %v\n", err)
	} else {
		fmt.Printf("  UNEXPECTED: draft with 'legacy' dialect validated OK\n")
	}

	fmt.Printf("\nALL CORE TESTS PASSED\n")
}
