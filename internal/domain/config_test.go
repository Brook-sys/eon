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
	hash, err := ConfigPayloadHash(draft.Scope, nil, nil, nil, draft.Interruption, nil)
	if err != nil || hash == "" {
		t.Fatalf("hash = %q err=%v", hash, err)
	}
	again, err := ConfigPayloadHash(draft.Scope, nil, nil, nil, draft.Interruption, nil)
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

func interruptionDraft(now time.Time) ConfigDraft {
	policy := DefaultInterruptionRuntimePolicy()
	return ConfigDraft{
		SchemaVersion: SchemaVersionV1, ID: "draft_1", Scope: ConfigScopeInterruption,
		BasedOnRevision: 0, Applicability: ConfigNextCycle, Status: ConfigDraftOpen,
		ActorType: ActorOperator, ActorID: "operator_1", Reason: "tune gate",
		Interruption: &policy, CreatedAt: now,
	}
}
