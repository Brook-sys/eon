package gatecampaign

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/provider/openai/fakeserver"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestMalformedRecoveryCampaignRunsFullLadderWithOneLiveCall(t *testing.T) {
	t.Parallel()
	manifest := MalformedRecoveryCampaignManifest{
		SchemaVersion: 1, Name: "test-malformed-recovery", TimeoutSeconds: 30,
		MaxCalls: 2, InjectedFailures: 1, MaxOutputTokens: 256,
		ProbePrompt: "Create the required observation.",
		Bindings: []RuntimeGateBinding{
			{Provider: "primary", ProviderKind: "openai_compatible", BindingID: "primary-binding", BaseURL: "http://primary", Model: "primary", APIKeyEnvironment: "PRIMARY_KEY", MaxOutputField: "max_tokens", ContextTokens: 8192, Priority: 1},
			{Provider: "fallback", ProviderKind: "openai_compatible", BindingID: "fallback-binding", BaseURL: "http://fallback", Model: "fallback", APIKeyEnvironment: "FALLBACK_KEY", MaxOutputField: "max_tokens", ContextTokens: 8192, Priority: 2},
		},
	}
	proposal := map[string]any{
		"schema_version": 1, "id": "changeset_malformed_recovery", "mission_revision_id": "revision_malformed_recovery",
		"operation_id": "operation_malformed_recovery", "base_commit_id": "commit_genesis", "read_set": []string{"manifest"},
		"preconditions": []string{}, "changes": []map[string]string{{"kind": "ADD", "entity_type": "observation", "entity_id": "observation_malformed_recovery", "payload_ref": "artifact_malformed_recovery"}},
		"expected_delta": "one observation", "validator_ids": []string{"schema"}, "provenance": "model:fallback", "idempotency_key": "malformed-recovery-campaign",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	server := fakeserver.New(fakeserver.Exchange{ResponseText: string(body), ResponseModel: "fallback"})
	defer server.Close()
	provider, err := newTestOpenAIProvider(server)
	if err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	report, err := (MalformedRecoveryCampaignRunner{Store: store, Clock: source.NewManualClock(time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)), Providers: map[string]port.ModelProvider{"primary-binding": provider, "fallback-binding": provider}}).Run(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if report.ExternalCalls != 1 || report.InjectedCalls != 1 || report.ModelCalls != 2 || len(server.Requests()) != 1 {
		t.Fatalf("unexpected bounds: report=%+v requests=%d", report, len(server.Requests()))
	}
	want := []string{"SHORT_CORRECTION"}
	if len(report.RecoveryStages) != len(want) {
		t.Fatalf("recovery stages=%v", report.RecoveryStages)
	}
	for i := range want {
		if string(report.RecoveryStages[i]) != want[i] {
			t.Fatalf("recovery stages=%v", report.RecoveryStages)
		}
	}
	if report.ReceiptCount != 2 || !report.CanonicalStored || report.OperationState != "SUCCEEDED" {
		t.Fatalf("durability snapshot=%+v", report)
	}
	if err := VerifyMalformedRecoveryDurability(context.Background(), store, report); err != nil {
		t.Fatal(err)
	}
}

func newTestOpenAIProvider(server *fakeserver.Server) (port.ModelProvider, error) {
	return openai.New(openai.Config{BaseURL: server.URL(), Model: "fallback", Client: server.Client()})
}
