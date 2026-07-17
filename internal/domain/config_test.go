package domain

import (
	"errors"
	"testing"
	"time"
)

func TestConfigDraftValidateAndHash(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	draft := interruptionDraft(now)
	if err := draft.Validate(); err != nil {
		t.Fatalf("valid draft: %v", err)
	}
	hash, err := ConfigPayloadHash(draft.Scope, nil, nil, nil, draft.Interruption, nil, nil)
	if err != nil || hash == "" {
		t.Fatalf("hash = %q err=%v", hash, err)
	}
	again, err := ConfigPayloadHash(draft.Scope, nil, nil, nil, draft.Interruption, nil, nil)
	if err != nil || again != hash {
		t.Fatalf("hash instability: %q vs %q err=%v", hash, again, err)
	}
	bad := draft
	bad.Runtime = &RuntimeProcessConfig{Version: "x", LogLevel: "info"}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected dual payload rejection")
	}
	immutable := draft
	immutable.Applicability = ConfigImmutable
	if err := immutable.Validate(); err == nil {
		t.Fatal("expected immutable draft rejection")
	}
	secret := SecretRef{Kind: "env", Name: "TELEGRAM_BOT_TOKEN"}
	if err := secret.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (SecretRef{Kind: "env", Name: "has space"}).Validate(); err == nil {
		t.Fatal("expected whitespace secret name rejection")
	}
}

func TestConfigDiffImpactAndApply(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	draft := interruptionDraft(now)
	diff, err := DiffConfig(nil, draft)
	if err != nil || diff.Empty {
		t.Fatalf("initial diff = %#v err=%v", diff, err)
	}
	impact, err := PreviewConfigImpact(draft, diff)
	if err != nil || impact.Blocked || !impact.NextCycleOnly {
		t.Fatalf("impact = %#v err=%v", impact, err)
	}
	validated, err := MarkConfigDraftValidated(draft, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	revision, applied, receipt, err := ApplyConfigDraft(nil, validated, "cfgrev_1", "receipt_cfg_1", now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if revision.Revision != 1 || applied.Status != ConfigDraftApplied || receipt.State != ConfigApplyApplied {
		t.Fatalf("apply = rev=%#v draft=%#v receipt=%#v", revision, applied, receipt)
	}
	// No-op second draft against active revision is blocked.
	noop := validated
	noop.ID = "draft_2"
	noop.BasedOnRevision = 1
	noop.Status = ConfigDraftOpen
	noop.ValidatedAt = time.Time{}
	noop.CreatedAt = now.Add(3 * time.Second)
	validatedNoop, err := MarkConfigDraftValidated(noop, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ApplyConfigDraft(&revision, validatedNoop, "cfgrev_2", "receipt_cfg_2", now.Add(5*time.Second)); err == nil {
		t.Fatal("expected no-op apply to fail")
	}
	// Stale base revision conflicts.
	changed := validated
	changed.ID = "draft_3"
	changed.BasedOnRevision = 0
	policy := *changed.Interruption
	policy.MaxPending = 9
	changed.Interruption = &policy
	if _, _, _, err := ApplyConfigDraft(&revision, changed, "cfgrev_3", "receipt_cfg_3", now.Add(6*time.Second)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale base error = %v", err)
	}
	// Successful second apply.
	nextDraft := validated
	nextDraft.ID = "draft_4"
	nextDraft.BasedOnRevision = 1
	nextPolicy := *nextDraft.Interruption
	nextPolicy.MaxPending = 5
	nextDraft.Interruption = &nextPolicy
	nextDiff, err := DiffConfig(&revision, nextDraft)
	if err != nil || nextDiff.Empty {
		t.Fatalf("next diff = %#v err=%v", nextDiff, err)
	}
	nextRev, _, _, err := ApplyConfigDraft(&revision, nextDraft, "cfgrev_4", "receipt_cfg_4", now.Add(7*time.Second))
	if err != nil || nextRev.Revision != 2 || nextRev.ParentID != revision.ID {
		t.Fatalf("next apply = %#v err=%v", nextRev, err)
	}
}

func TestConfigApplyReceiptMonotonic(t *testing.T) {
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	current := ConfigApplyReceipt{
		SchemaVersion: SchemaVersionV1, ID: "receipt_1", DraftID: "draft_1",
		State: ConfigApplyReceived, RecordedAt: now,
	}
	validating := current
	validating.State, validating.RecordedAt = ConfigApplyValidating, now.Add(time.Second)
	if err := AdvanceConfigApplyReceipt(current, validating); err != nil {
		t.Fatal(err)
	}
	applied := validating
	applied.State, applied.RevisionID, applied.ResultRef, applied.RecordedAt = ConfigApplyApplied, "cfgrev_1", "INTERRUPTION@1", now.Add(2*time.Second)
	if err := AdvanceConfigApplyReceipt(validating, applied); !errors.Is(err, ErrConflict) {
		t.Fatalf("skip transition error = %v", err)
	}
	accepted := validating
	accepted.State, accepted.RecordedAt = ConfigApplyAccepted, now.Add(2*time.Second)
	if err := AdvanceConfigApplyReceipt(validating, accepted); err != nil {
		t.Fatal(err)
	}
	applying := accepted
	applying.State, applying.RecordedAt = ConfigApplyApplying, now.Add(3*time.Second)
	if err := AdvanceConfigApplyReceipt(accepted, applying); err != nil {
		t.Fatal(err)
	}
	done := applying
	done.State, done.RevisionID, done.ResultRef, done.RecordedAt = ConfigApplyApplied, "cfgrev_1", "INTERRUPTION@1", now.Add(4*time.Second)
	if err := AdvanceConfigApplyReceipt(applying, done); err != nil {
		t.Fatal(err)
	}
	reverse := done
	reverse.State, reverse.RevisionID, reverse.ResultRef, reverse.RecordedAt = ConfigApplyApplying, "", "", now.Add(5*time.Second)
	if err := AdvanceConfigApplyReceipt(done, reverse); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal reverse error = %v", err)
	}
}

func TestChannelsConfigDiffRedactsSecrets(t *testing.T) {
	now := time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC)
	base := ConfigDraft{
		SchemaVersion: SchemaVersionV1, ID: "draft_ch_1", Scope: ConfigScopeChannels,
		Applicability: ConfigHot, Status: ConfigDraftOpen, ActorType: ActorOperator, ActorID: "op",
		Reason: "enable telegram", CreatedAt: now,
		Channels: &ChannelsConfig{
			Version: "channels.v1",
			Routes: []ChannelRouteConfig{{
				Channel: "telegram", DestinationRef: "operator_primary", Enabled: true, Priority: 10,
				CredentialRef: SecretRef{Kind: "env", Name: "TELEGRAM_BOT_TOKEN"}, MaxDeliveriesPH: 30,
			}},
		},
	}
	diff, err := DiffConfig(nil, base)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, change := range diff.Changes {
		if change.Path == "channels.routes[0].credential_ref" {
			found = true
			if !change.Secret || change.After != "[secret-ref]" {
				t.Fatalf("secret change = %#v", change)
			}
		}
	}
	if !found {
		t.Fatalf("missing secret path in %#v", diff.Changes)
	}
}

func TestDraftFromConfigRevisionRollback(t *testing.T) {
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	draft := interruptionDraft(now)
	validated, err := MarkConfigDraftValidated(draft, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	rev1, _, _, err := ApplyConfigDraft(nil, validated, "cfgrev_1", "receipt_cfg_1", now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	// Second revision changes MaxPending.
	next := validated
	next.ID = "draft_2"
	next.BasedOnRevision = 1
	next.Status = ConfigDraftOpen
	next.ValidatedAt = time.Time{}
	next.CreatedAt = now.Add(3 * time.Second)
	pol := *next.Interruption
	pol.MaxPending = 9
	next.Interruption = &pol
	nextValidated, err := MarkConfigDraftValidated(next, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	rev2, _, _, err := ApplyConfigDraft(&rev1, nextValidated, "cfgrev_2", "receipt_cfg_2", now.Add(5*time.Second))
	if err != nil || rev2.Revision != 2 {
		t.Fatalf("rev2 = %#v err=%v", rev2, err)
	}
	// Pure rollback draft from rev1 against active rev2.
	rollbackDraft, err := DraftFromConfigRevision(rev1, "draft_rb", 2, ActorOperator, "operator_1", "restore rev1 payload", now.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rollbackDraft.Status != ConfigDraftOpen || rollbackDraft.BasedOnRevision != 2 {
		t.Fatalf("rollback draft = %#v", rollbackDraft)
	}
	if ConfigRevisionsEqualPayload(rev1, rev2) {
		t.Fatal("rev1 and rev2 should differ")
	}
	rbValidated, err := MarkConfigDraftValidated(rollbackDraft, now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	rev3, applied, receipt, err := ApplyConfigDraft(&rev2, rbValidated, "cfgrev_3", "receipt_cfg_3", now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rev3.Revision != 3 || rev3.ParentID != rev2.ID || !ConfigRevisionsEqualPayload(rev3, rev1) {
		t.Fatalf("rev3 = %#v want payload of rev1", rev3)
	}
	if applied.Status != ConfigDraftApplied || receipt.State != ConfigApplyApplied {
		t.Fatalf("applied=%#v receipt=%#v", applied, receipt)
	}
	// No-op restore of active payload is blocked by impact preview.
	noopDraft, err := DraftFromConfigRevision(rev3, "draft_noop_rb", 3, ActorOperator, "operator_1", "noop", now.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	noopValidated, err := MarkConfigDraftValidated(noopDraft, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ApplyConfigDraft(&rev3, noopValidated, "cfgrev_4", "receipt_cfg_4", now.Add(11*time.Second)); err == nil {
		t.Fatal("expected no-op rollback apply to fail")
	}
}

func interruptionDraft(now time.Time) ConfigDraft {
	policy := DefaultInterruptionRuntimePolicy()
	return ConfigDraft{
		SchemaVersion: SchemaVersionV1, ID: "draft_1", Scope: ConfigScopeInterruption,
		BasedOnRevision: 0, Applicability: ConfigNextCycle, Status: ConfigDraftOpen,
		ActorType: ActorOperator, ActorID: "operator_1", Reason: "tune gate",
		Interruption: &policy, CreatedAt: now,
	}
}

func TestDefaultSchedulerCadenceConfigAndWithinCycleBudget(t *testing.T) {
	def := DefaultSchedulerCadenceConfig()
	if err := def.Validate(); err != nil {
		t.Fatalf("default cadence: %v", err)
	}
	if def.MaxDispatches <= 0 || def.MaxCycleDuration <= 0 {
		t.Fatalf("default cadence should bound dispatches and duration: %#v", def)
	}
	zero := def
	zero.MaxDispatches = 0
	if err := zero.Validate(); err == nil {
		t.Fatal("expected zero max dispatches rejection")
	}
	started := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	if !WithinCycleBudget(started, started.Add(time.Second), 5*time.Second) {
		t.Fatal("expected inside budget")
	}
	if WithinCycleBudget(started, started.Add(5*time.Second), 5*time.Second) {
		t.Fatal("exact boundary must be exhausted")
	}
	if !WithinCycleBudget(started, started.Add(time.Hour), 0) {
		t.Fatal("zero max disables deadline")
	}
}

func TestModelsConfigDraftDiffAndRollback(t *testing.T) {
	now := time.Date(2026, 7, 17, 2, 0, 0, 0, time.UTC)
	provider := ModelProviderConfig{
		ID: "groq", Kind: ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1",
		APIKeyEnv: "GROQ_API_KEY", Timeout: 30 * time.Second, MaxResponseBytes: 1 << 20,
		GlobalLimit: ResourceLimit{Resource: ModelProviderResource("groq"), MaxConcurrent: 1},
	}
	binding := ModelBindingConfig{
		ID: "groq-gemma", ProviderRef: "groq", ModelID: "gemma2-9b-it", Enabled: true,
		Priority: 10, ContextTokens: 8192, MaxOutputTokens: 1024,
		MaxOutputDialect: MaxOutputDialectCompletion,
		Limit:            ResourceLimit{Resource: ModelBindingResource("groq-gemma"), MaxConcurrent: 1},
	}
	draft := ConfigDraft{
		SchemaVersion: SchemaVersionV1, ID: "draft_models_1", Scope: ConfigScopeModels,
		Applicability: ConfigNextCycle, Status: ConfigDraftOpen, ActorType: ActorOperator,
		ActorID: "operator", Reason: "configure providers", CreatedAt: now,
		Models: &ModelsConfig{Version: "models.v1", Providers: []ModelProviderConfig{provider}, Bindings: []ModelBindingConfig{binding}},
	}
	if err := draft.Validate(); err != nil {
		t.Fatal(err)
	}
	diff, err := DiffConfig(nil, draft)
	if err != nil {
		t.Fatal(err)
	}
	foundSecretRef := false
	for _, change := range diff.Changes {
		if change.Path == "models.providers[0].api_key_env" {
			foundSecretRef = change.Secret && change.After == "[secret-ref]"
		}
	}
	if !foundSecretRef {
		t.Fatalf("missing redacted api key env in %#v", diff.Changes)
	}
	validated, err := MarkConfigDraftValidated(draft, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	rev, _, _, err := ApplyConfigDraft(nil, validated, "cfgrev_models_1", "receipt_models_1", now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := DraftFromConfigRevision(rev, "draft_models_rb", 1, ActorOperator, "operator", "restore model catalog", now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Models == nil || len(rollback.Models.Bindings) != 1 || rollback.Models.Bindings[0].ID != binding.ID {
		t.Fatalf("rollback models = %#v", rollback.Models)
	}
	rollback.Models.Bindings[0].ID = "changed"
	if rev.Models.Bindings[0].ID != binding.ID {
		t.Fatal("rollback must deep-copy model bindings")
	}
}
