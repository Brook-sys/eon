package inspect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestStoreRetentionProjectionAndHTTP(t *testing.T) {
	t.Parallel()
	store := memory.New()
	now := time.Date(2026, 7, 16, 18, 0, 0, 0, time.UTC)
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		// Stale regenerable-shaped cited view without backing claim → non-regenerable.
		if err := tx.AppendKnowledgeArtifact(domain.KnowledgeArtifact{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "artifact_stale_cited",
			Kind:          "cited_claim_view",
			BaseCommitID:  domain.GenesisCommitID,
			Dependencies:  []string{domain.FormatClaimDependency("claim_missing", 1)},
			ContentRef:    "sha256:a",
			Content:       "stale cited",
			Stale:         true,
		}); err != nil {
			return err
		}
		// Stale non-cited kind.
		if err := tx.AppendKnowledgeArtifact(domain.KnowledgeArtifact{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "artifact_stale_other",
			Kind:          "summary_view",
			BaseCommitID:  domain.GenesisCommitID,
			Dependencies:  []string{"claim:x@1"},
			ContentRef:    "sha256:b",
			Content:       "other",
			Stale:         true,
		}); err != nil {
			return err
		}
		// Fresh artifact ignored by planner.
		return tx.AppendKnowledgeArtifact(domain.KnowledgeArtifact{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "artifact_fresh",
			Kind:          "cited_claim_view",
			BaseCommitID:  domain.GenesisCommitID,
			Dependencies:  []string{domain.FormatClaimDependency("claim_x", 1)},
			ContentRef:    "sha256:c",
			Content:       "fresh",
			Stale:         false,
		})
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "test", Version: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	proj, err := projector.StoreRetention(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if proj.PolicyVersion != domain.StoreRetentionPolicyVersion {
		t.Fatalf("policy = %q", proj.PolicyVersion)
	}
	if proj.AllowEventLogPrune {
		t.Fatal("event log prune must be false")
	}
	if proj.StaleArtifactCount != 2 {
		t.Fatalf("stale = %d, want 2", proj.StaleArtifactCount)
	}
	if proj.RefreshCandidateCount != 2 {
		t.Fatalf("candidates = %d, want 2", proj.RefreshCandidateCount)
	}
	if proj.RegenerableCitedViews != 0 {
		t.Fatalf("regenerable = %d, want 0 (missing claim)", proj.RegenerableCitedViews)
	}
	if len(proj.AuthorizedActions) != 3 {
		t.Fatalf("authorized = %v", proj.AuthorizedActions)
	}
	foundRefresh := false
	for _, a := range proj.AuthorizedActions {
		if a == string(domain.RetentionActionRefreshCandidates) {
			foundRefresh = true
		}
		if a == string(domain.RetentionActionEventLogPrune) {
			t.Fatal("event_log_prune must not appear in authorized actions")
		}
	}
	if !foundRefresh {
		t.Fatal("missing refresh_candidates authorization")
	}

	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/store/retention")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body inspect.StoreRetentionProjection
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.StaleArtifactCount != 2 || body.PolicyVersion != domain.StoreRetentionPolicyVersion {
		t.Fatalf("http body = %+v", body)
	}
}
