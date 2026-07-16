package kernel

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestConfigApplierValidateAndApply(t *testing.T) {
	store := memory.New()
	clock := source.NewManualClock(time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC))
	ids := source.NewSequenceIDGenerator(1)
	applier, err := NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	policy := domain.DefaultInterruptionRuntimePolicy()
	draft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_int_1", Scope: domain.ConfigScopeInterruption,
		BasedOnRevision: 0, Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "operator_1", Reason: "baseline gate",
		Interruption: &policy, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(draft)
	}); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	preview, diff, err := applier.ValidateDraft(context.Background(), draft.ID)
	if err != nil || preview.Blocked || diff.Empty {
		t.Fatalf("validate = preview=%#v diff=%#v err=%v", preview, diff, err)
	}
	clock.Advance(time.Second)
	revision, receipt, err := applier.ApplyDraft(context.Background(), draft.ID)
	if err != nil || revision.Revision != 1 || receipt.State != domain.ConfigApplyApplied {
		t.Fatalf("apply = rev=%#v receipt=%#v err=%v", revision, receipt, err)
	}
	// Replay is pure.
	again, againReceipt, err := applier.ApplyDraft(context.Background(), draft.ID)
	if err != nil || again.ID != revision.ID || againReceipt.State != domain.ConfigApplyApplied {
		t.Fatalf("replay = %#v receipt=%#v err=%v", again, againReceipt, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveConfigRevision(domain.ConfigScopeInterruption)
		if err != nil || active.ID != revision.ID {
			t.Fatalf("active = %#v err=%v", active, err)
		}
		gotDraft, err := r.ConfigDraft(draft.ID)
		if err != nil || gotDraft.Status != domain.ConfigDraftApplied {
			t.Fatalf("draft = %#v err=%v", gotDraft, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Second revision with change.
	clock.Advance(time.Second)
	nextPolicy := policy
	nextPolicy.MaxPending = 6
	next := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_int_2", Scope: domain.ConfigScopeInterruption,
		BasedOnRevision: 1, Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "operator_1", Reason: "raise pending",
		Interruption: &nextPolicy, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(next)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), next.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	rev2, _, err := applier.ApplyDraft(context.Background(), next.ID)
	if err != nil || rev2.Revision != 2 || rev2.ParentID != revision.ID {
		t.Fatalf("second apply = %#v err=%v", rev2, err)
	}
}

func TestConfigApplierRejectsNoopAndStale(t *testing.T) {
	store := memory.New()
	clock := source.NewManualClock(time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC))
	ids := source.NewSequenceIDGenerator(100)
	applier, err := NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	policy := domain.DefaultInterruptionRuntimePolicy()
	first := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_a", Scope: domain.ConfigScopeInterruption,
		Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen, ActorType: domain.ActorOperator,
		ActorID: "op", Reason: "seed", Interruption: &policy, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(first)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, _, err := applier.ApplyDraft(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	// No-op draft is rejected at validate.
	clock.Advance(time.Second)
	noop := first
	noop.ID = "draft_noop"
	noop.BasedOnRevision = 1
	noop.Status = domain.ConfigDraftOpen
	noop.ValidatedAt = time.Time{}
	noop.CreatedAt = clock.Now()
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(noop)
	}); err != nil {
		t.Fatal(err)
	}
	preview, _, err := applier.ValidateDraft(context.Background(), noop.ID)
	if err != nil || !preview.Blocked {
		t.Fatalf("noop preview = %#v err=%v", preview, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		draft, err := r.ConfigDraft(noop.ID)
		if err != nil || draft.Status != domain.ConfigDraftRejected {
			t.Fatalf("noop draft = %#v err=%v", draft, err)
		}
		receipt, err := r.ConfigApplyReceipt(noop.ID)
		if err != nil || receipt.State != domain.ConfigApplyRejected {
			t.Fatalf("noop receipt = %#v err=%v", receipt, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Stale base fails at apply after validate.
	clock.Advance(time.Second)
	changed := policy
	changed.MaxPending = 8
	stale := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_stale", Scope: domain.ConfigScopeInterruption,
		BasedOnRevision: 0, Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "op", Reason: "stale", Interruption: &changed, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(stale)
	}); err != nil {
		t.Fatal(err)
	}
	// Validate against active will produce a non-empty diff, but base revision 0 is wrong.
	// DiffConfig doesn't check base; Apply does. Validate may still accept.
	if _, _, err := applier.ValidateDraft(context.Background(), stale.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, _, err := applier.ApplyDraft(context.Background(), stale.ID); err == nil || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected stale apply ErrConflict, got %v", err)
	}
}
