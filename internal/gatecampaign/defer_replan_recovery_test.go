package gatecampaign

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/port"

	"motor-autonomo/internal/provider/openai/fakeserver"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/storage/sqlite"
)

func TestDeferReplanRecoveryCampaignSwitchesBindingWithOneExternalCall(t *testing.T) {
	t.Parallel()
	manifest := DeferReplanRecoveryCampaignManifest{
		SchemaVersion: 1, Name: "test-defer-replan-recovery", TimeoutSeconds: 30,
		MaxCalls: 4, InjectedFailures: 3, MaxOutputTokens: 256,
		ProbePrompt: "Create the required observation.",
		Bindings: []RuntimeGateBinding{
			{Provider: "primary", ProviderKind: "openai_compatible", BindingID: "primary-binding", BaseURL: "http://primary", Model: "primary", APIKeyEnvironment: "PRIMARY_KEY", MaxOutputField: "max_tokens", ContextTokens: 8192, Priority: 1},
			{Provider: "fallback", ProviderKind: "openai_compatible", BindingID: "fallback-binding", BaseURL: "http://fallback", Model: "fallback", APIKeyEnvironment: "FALLBACK_KEY", MaxOutputField: "max_tokens", ContextTokens: 8192, Priority: 2},
		},
	}
	server := fakeserver.New(fakeserver.Exchange{ResponseText: `{"live":"transport evidence only"}`, ResponseModel: "fallback"})
	defer server.Close()
	provider, err := newTestOpenAIProvider(server)
	if err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	report, err := (DeferReplanRecoveryCampaignRunner{Store: store, Clock: source.NewManualClock(time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)), Providers: map[string]port.ModelProvider{"primary-binding": provider, "fallback-binding": provider}}).Run(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if report.ExternalCalls != 1 || report.InjectedCalls != 3 || report.ModelCalls != 4 || len(server.Requests()) != 1 {
		t.Fatalf("unexpected bounds: report=%+v requests=%d", report, len(server.Requests()))
	}
	want := []string{"SHORT_CORRECTION", "SIMPLER_FORMAT", "FALLBACK_MODEL", "DEFER"}
	if len(report.RecoveryStages) != len(want) {
		t.Fatalf("recovery stages=%v", report.RecoveryStages)
	}
	for i := range want {
		if string(report.RecoveryStages[i]) != want[i] {
			t.Fatalf("recovery stages=%v", report.RecoveryStages)
		}
	}
	if report.ReceiptCount != 4 || !report.CanonicalAbsent || report.OperationState != "READY" || report.OperationAttempt != 1 || report.FinalDisposition != "REPLAN" || report.DecisionReason != "intra_execute_recovery_exhausted_replan_allowed" || report.ReplanEvents != 1 || report.ModelFailedEvents != 1 || report.ExhaustionEvents != 0 {
		t.Fatalf("durability snapshot=%+v", report)
	}
	if !report.BindingSwitched || report.PrimaryBinding != "primary-binding" || report.FallbackBinding != "fallback-binding" {
		t.Fatalf("binding switch=%+v", report)
	}
	for i := 0; i < 3; i++ {
		if !report.Calls[i].Injected || report.Calls[i].BindingID != "primary-binding" {
			t.Fatalf("primary injected call %d=%+v", i+1, report.Calls[i])
		}
	}
	if report.Calls[3].Injected || !report.Calls[3].PresentedInvalid || report.Calls[3].PresentedHash == report.Calls[3].ResponseHash || report.Calls[3].BindingID != "fallback-binding" {
		t.Fatalf("external fallback call=%+v", report.Calls[3])
	}
	if err := VerifyDeferReplanRecoveryDurability(context.Background(), store, report); err != nil {
		t.Fatal(err)
	}
}

func TestDeferReplanRecoveryCampaignSQLiteReopenVerifiesTerminalState(t *testing.T) {
	manifest := DeferReplanRecoveryCampaignManifest{
		SchemaVersion: 1, Name: "test-defer-replan-recovery-sqlite", TimeoutSeconds: 30,
		MaxCalls: 4, InjectedFailures: 3, MaxOutputTokens: 256,
		ProbePrompt: "Create the required observation.",
		Bindings: []RuntimeGateBinding{
			{Provider: "primary", ProviderKind: "openai_compatible", BindingID: "primary-binding", BaseURL: "http://primary", Model: "primary", APIKeyEnvironment: "PRIMARY_KEY", MaxOutputField: "max_tokens", ContextTokens: 8192, Priority: 1},
			{Provider: "fallback", ProviderKind: "openai_compatible", BindingID: "fallback-binding", BaseURL: "http://fallback", Model: "fallback", APIKeyEnvironment: "FALLBACK_KEY", MaxOutputField: "max_tokens", ContextTokens: 8192, Priority: 2},
		},
	}
	server := fakeserver.New(fakeserver.Exchange{ResponseText: `{"status":"live-ok"}`, ResponseModel: "fallback"})
	defer server.Close()
	provider, err := newTestOpenAIProvider(server)
	if err != nil {
		t.Fatal(err)
	}
	databasePath := t.TempDir() + "/defer-replan-recovery.sqlite"
	store, err := sqlite.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	report, err := (DeferReplanRecoveryCampaignRunner{Store: store, Clock: source.NewManualClock(time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)), Providers: map[string]port.ModelProvider{"primary-binding": provider, "fallback-binding": provider}}).Run(context.Background(), manifest)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := VerifyDeferReplanRecoveryDurability(context.Background(), reopened, report); err != nil {
		t.Fatal(err)
	}
	if report.ReceiptCount != 4 || !report.CanonicalAbsent || report.OperationState != "READY" || report.OperationAttempt != 1 || report.FinalDisposition != "REPLAN" || report.ReplanEvents != 1 || report.ModelFailedEvents != 1 || report.ExhaustionEvents != 0 {
		t.Fatalf("incomplete durable report: %+v", report)
	}
}
