package kernel

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestPlanChildDraftsSplitsStructuralGapsAndCapsFanOut(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 19, 0, 0, 0, time.UTC)
	store := memory.New()
	seedMission(t, store)
	body := []byte("body")
	hash := "sha256:230d8358dc8e8890b4c58deeb62912ee2f20357ae92a5cc861b98e68fe31acb5"
	// Three orthogonal gap dimensions:
	// - source_no_frag: without_frag + without_obs
	// - source_gap_frag: fragment without observation (frags_without_obs + without_obs)
	if err := store.Update(ctx, func(tx port.Transaction) error {
		noFrag := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_no_frag", Kind: "fixture", Locator: "empty.txt", ObservedAt: now}
		gapFrag := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_gap_frag", Kind: "fixture", Locator: "gap.txt", ObservedAt: now}
		verN := domain.SourceVersion{SchemaVersion: domain.SchemaVersionV1, ID: "sv_nofrag", SourceID: noFrag.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		verG := domain.SourceVersion{SchemaVersion: domain.SchemaVersionV1, ID: "sv_gap", SourceID: gapFrag.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		snapN := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: verN.ID, MediaType: "text/plain", Content: body}
		snapG := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: verG.ID, MediaType: "text/plain", Content: body}
		fragG := domain.SourceFragment{SchemaVersion: domain.SchemaVersionV1, ID: "frag_gap", SourceVersionID: verG.ID, Location: "bytes:0-4", StartOffset: 0, EndOffset: 4, ContentHash: hash, ContentRef: hash}
		for _, step := range []error{
			tx.AppendSource(noFrag, verN, snapN),
			tx.AppendSource(gapFrag, verG, snapG),
			tx.AppendSourceFragments(verG.ID, []domain.SourceFragment{fragG}),
		} {
			if step != nil {
				return step
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	policy := domain.DefaultHorizonPolicy()
	full, err := PlanChildDraftsFromStoreWithPolicy(ctx, store, domain.FamilyGapScan, "revision_1", now, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 3 {
		t.Fatalf("expected 3 gap splits, got %+v", full)
	}
	wantSigs := []string{"gap:without_frag", "gap:without_obs", "gap:frags_without_obs"}
	for i, want := range wantSigs {
		if full[i].DedupSignature != want {
			t.Fatalf("draft[%d] = %q, want %q", i, full[i].DedupSignature, want)
		}
		if full[i].Origin == "" || full[i].Novelty == "" || full[i].StopCondition == "" {
			t.Fatalf("draft[%d] incomplete: %+v", i, full[i])
		}
	}

	// HorizonPolicy max_children caps fan-out before SpawnChildren rejects.
	policy.MaxChildren = 1
	capped, err := PlanChildDraftsFromStoreWithPolicy(ctx, store, domain.FamilyGapScan, "revision_1", now, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(capped) != 1 || capped[0].DedupSignature != "gap:without_frag" {
		t.Fatalf("capped drafts = %+v", capped)
	}

	// Coverage gains the same structural splits (claims_without_ev absent here).
	cov, err := PlanChildDraftsFromStoreWithPolicy(ctx, store, domain.FamilyCoverageScan, "revision_1", now, domain.DefaultHorizonPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(cov) != 3 {
		t.Fatalf("coverage splits = %+v", cov)
	}
	if cov[0].DedupSignature != "coverage:without_frag" || cov[2].DedupSignature != "coverage:frags_without_obs" {
		t.Fatalf("coverage signatures = %+v", cov)
	}

	// resolveChildDrafts must pass policy through for configured static drafts too.
	staticOnly, err := resolveChildDrafts(ctx, memory.New(), domain.FamilyGapScan, "revision_1", now, full, policy)
	if err != nil {
		t.Fatal(err)
	}
	// empty store plan → uses configured (full) but capped by MaxChildren=1
	if len(staticOnly) != 1 {
		t.Fatalf("resolve with configured+cap = %+v", staticOnly)
	}
}
