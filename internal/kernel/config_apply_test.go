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

func TestConfigApplierSemanticRollback(t *testing.T) {
	store := memory.New()
	clock := source.NewManualClock(time.Date(2026, 7, 16, 17, 0, 0, 0, time.UTC))
	ids := source.NewSequenceIDGenerator(200)
	applier, err := NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	policy := domain.DefaultInterruptionRuntimePolicy()
	seed := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_rb_seed", Scope: domain.ConfigScopeInterruption,
		Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen, ActorType: domain.ActorOperator,
		ActorID: "op", Reason: "seed", Interruption: &policy, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(seed)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), seed.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	rev1, _, err := applier.ApplyDraft(context.Background(), seed.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Advance to rev2 with different payload.
	clock.Advance(time.Second)
	nextPolicy := policy
	nextPolicy.MaxPending = 11
	second := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1, ID: "draft_rb_second", Scope: domain.ConfigScopeInterruption,
		BasedOnRevision: 1, Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "op", Reason: "raise pending", Interruption: &nextPolicy, CreatedAt: clock.Now(),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateConfigDraft(second)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(context.Background(), second.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	rev2, _, err := applier.ApplyDraft(context.Background(), second.ID)
	if err != nil || rev2.Revision != 2 {
		t.Fatalf("rev2 = %#v err=%v", rev2, err)
	}
	// Semantic rollback to rev1 payload → new rev3.
	clock.Advance(time.Second)
	draft, rev3, receipt, err := applier.RollbackToRevision(context.Background(), domain.ConfigScopeInterruption, rev1.ID, domain.ActorOperator, "op", "restore seed policy")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rev3.Revision != 3 || rev3.ParentID != rev2.ID || receipt.State != domain.ConfigApplyApplied || draft.Status != domain.ConfigDraftApplied {
		t.Fatalf("rollback result draft=%#v rev=%#v receipt=%#v", draft, rev3, receipt)
	}
	if !domain.ConfigRevisionsEqualPayload(rev3, rev1) || rev3.Interruption == nil || rev3.Interruption.MaxPending != policy.MaxPending {
		t.Fatalf("rev3 payload mismatch: %#v vs %#v", rev3, rev1)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveConfigRevision(domain.ConfigScopeInterruption)
		if err != nil || active.ID != rev3.ID {
			t.Fatalf("active = %#v err=%v", active, err)
		}
		revisions, err := r.ConfigRevisions(domain.ConfigScopeInterruption)
		if err != nil || len(revisions) != 3 {
			t.Fatalf("revisions = %#v err=%v", revisions, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// No-op rollback of already-active payload is conflict.
	clock.Advance(time.Second)
	if _, _, _, err := applier.RollbackToRevision(context.Background(), domain.ConfigScopeInterruption, rev3.ID, domain.ActorOperator, "op", "noop"); err == nil || !errors.Is(err, port.ErrConflict) {
		t.Fatalf("expected no-op conflict, got %v", err)
	}
	// Rollback to non-ancestor (same or future number without being earlier) fails.
	// rev2 is earlier than rev3 and has different payload → allowed; trying rev3 target when active is rev3 already covered.
	// Target with wrong scope fails.
	if _, _, _, err := applier.RollbackToRevision(context.Background(), domain.ConfigScopeHorizon, rev1.ID, domain.ActorOperator, "op", "wrong scope"); err == nil {
		t.Fatal("expected scope conflict")
	}
}
