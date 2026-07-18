package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShippedModelPresetCatalogMatchesEvidence(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "model-presets.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	catalog, err := DecodeModelPresetCatalog(file, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, preset := range catalog.Presets {
		evidence, err := os.Open(filepath.Join("..", "..", filepath.FromSlash(preset.EvidenceReport)))
		if err != nil {
			t.Fatalf("preset %s evidence: %v", preset.ID, err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, io.LimitReader(evidence, 16<<20))
		closeErr := evidence.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatalf("preset %s evidence read: copy=%v close=%v", preset.ID, copyErr, closeErr)
		}
		if got := hex.EncodeToString(hash.Sum(nil)); got != preset.EvidenceSHA256 {
			t.Fatalf("preset %s evidence digest=%s want=%s", preset.ID, got, preset.EvidenceSHA256)
		}
	}
}

func validModelPreset() ModelPreset {
	return ModelPreset{
		ID: "groq-llama-3.3-70b", ObservedAt: time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC), Qualification: "QUALIFIED",
		EvidenceReport: "results/model-benchmark/live-groq-llama-3.3-70b-contract-2026-07-18/report.json",
		EvidenceSHA256: "d4d6017ebc091118a53c1cd0054322d1b4294843afaf565f5e9105917168fab5", RecommendedPriority: 10,
		Provider: ModelProviderConfig{ID: "groq", Kind: ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "GROQ_API_KEY", Timeout: 90 * time.Second, MaxResponseBytes: 1 << 20, GlobalLimit: ResourceLimit{Resource: ModelProviderResource("groq"), MaxConcurrent: 1, CooldownBase: 30 * time.Second, CooldownMax: 5 * time.Minute}},
		Binding:  ModelBindingConfig{ID: "groq-llama-3.3-70b", ProviderRef: "groq", ModelID: "llama-3.3-70b-versatile", Enabled: false, ContextTokens: 8192, MaxOutputTokens: 512, MaxOutputDialect: MaxOutputDialectLegacy, Limit: ResourceLimit{Resource: ModelBindingResource("groq-llama-3.3-70b"), MaxConcurrent: 1, CooldownBase: 30 * time.Second, CooldownMax: 5 * time.Minute}},
	}
}

func TestDecodeModelPresetCatalogIsStrictAndBounded(t *testing.T) {
	preset := validModelPreset()
	body := `{"schema":"model-presets.v1","presets":[{"id":"` + preset.ID + `","provider":{"id":"groq","kind":"groq","base_url":"https://api.groq.com/openai/v1","api_key_env":"GROQ_API_KEY","timeout":90000000000,"max_response_bytes":1048576,"global_limit":{"resource":"model-provider:groq","max_concurrent":1,"max_per_minute":0,"max_per_day":0,"max_tokens_per_minute":0,"failure_threshold":3,"cooldown_base":30000000000,"cooldown_max":300000000000,"reserved_for_critical":0}},"binding":{"id":"groq-llama-3.3-70b","provider_ref":"groq","model_id":"llama-3.3-70b-versatile","enabled":false,"priority":10,"context_tokens":8192,"max_output_tokens":512,"max_output_dialect":"max_tokens","limit":{"resource":"model-binding:groq-llama-3.3-70b","max_concurrent":1,"max_per_minute":0,"max_per_day":0,"max_tokens_per_minute":0,"failure_threshold":3,"cooldown_base":30000000000,"cooldown_max":300000000000,"reserved_for_critical":0}},"observed_at":"2026-07-18T00:00:00Z","qualification":"QUALIFIED","evidence_report":"` + preset.EvidenceReport + `","evidence_sha256":"` + preset.EvidenceSHA256 + `","recommended_priority":10}]}`
	if _, err := DecodeModelPresetCatalog(strings.NewReader(body), int64(len(body))); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeModelPresetCatalog(strings.NewReader(body+` {}`), int64(len(body)+3)); err == nil {
		t.Fatal("expected trailing JSON to fail")
	}
	if _, err := DecodeModelPresetCatalog(strings.NewReader(body), int64(len(body)-1)); err == nil {
		t.Fatal("expected byte bound to fail")
	}
}

func TestModelPresetRequiresQualifiedEvidenceAndStaysDisabled(t *testing.T) {
	preset := validModelPreset()
	config, err := preset.ModelsConfigDraft("models.from-preset.v1")
	if err != nil {
		t.Fatal(err)
	}
	if config.Bindings[0].Enabled {
		t.Fatal("preset must not enable binding")
	}
	if config.Bindings[0].Priority != preset.RecommendedPriority {
		t.Fatalf("priority=%d", config.Bindings[0].Priority)
	}

	preset.Qualification = "DEGRADED"
	if err := preset.Validate(); err == nil {
		t.Fatal("expected degraded evidence to fail")
	}
}

func TestModelPresetRejectsUnsafeEvidenceAndEnabledBinding(t *testing.T) {
	for _, mutate := range []func(*ModelPreset){
		func(p *ModelPreset) { p.EvidenceReport = "../secret" },
		func(p *ModelPreset) { p.EvidenceSHA256 = "ABC" },
		func(p *ModelPreset) { p.Binding.Enabled = true },
	} {
		preset := validModelPreset()
		mutate(&preset)
		if err := preset.Validate(); err == nil {
			t.Fatal("expected invalid preset")
		}
	}
}

func TestModelPresetEnablementPreviewRequiresExactDisabledInstallation(t *testing.T) {
	preset := validModelPreset()
	installed, err := preset.ModelsConfigDraft("models.installed.v1")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := preset.PreviewEnablement(&installed, "models.enabled.v1")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Blocked || preview.Candidate == nil || !preview.Candidate.Bindings[0].Enabled {
		t.Fatalf("enablement preview = %#v", preview)
	}
	if len(preview.Risks) < 4 || preview.EvidenceSHA256 != preset.EvidenceSHA256 {
		t.Fatalf("risk/evidence projection = %#v", preview)
	}

	missing, err := preset.PreviewEnablement(nil, "models.enabled.v1")
	if err != nil || !missing.Blocked || missing.Candidate != nil {
		t.Fatalf("missing active preview = %#v err=%v", missing, err)
	}
	drifted := installed
	drifted.Bindings = append([]ModelBindingConfig(nil), installed.Bindings...)
	drifted.Bindings[0].MaxOutputTokens++
	driftedPreview, err := preset.PreviewEnablement(&drifted, "models.enabled.v1")
	if err != nil || !driftedPreview.Blocked || driftedPreview.Candidate != nil {
		t.Fatalf("drifted preview = %#v err=%v", driftedPreview, err)
	}
}
