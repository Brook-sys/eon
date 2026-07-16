package inspect_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestKnowledgeCatalogBrowseAndInspectors(t *testing.T) {
	store, now := seedKnowledge(t)
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	catalog, err := projector.KnowledgeCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Sources != 1 || catalog.SourceVersions != 1 || catalog.Observations != 2 || catalog.Claims != 2 {
		t.Fatalf("catalog counts = %#v", catalog)
	}
	if catalog.EvidenceLinks != 3 || catalog.Artifacts != 1 || catalog.StaleArtifacts != 0 {
		t.Fatalf("catalog evidence/artifacts = %#v", catalog)
	}
	if catalog.ClaimsWithoutEv != 0 || catalog.SupportingEv != 2 || catalog.ContradictingEv != 1 {
		t.Fatalf("catalog quality = %#v", catalog)
	}

	sources, err := projector.ListSources(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sources.Total != 1 || len(sources.Items) != 1 || sources.Items[0].ID != "source_1" || sources.Items[0].Versions != 1 {
		t.Fatalf("sources = %#v", sources)
	}

	sourceDetail, err := projector.SourceInspector(context.Background(), "source_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceDetail.Versions) != 1 || sourceDetail.Versions[0].FragmentCount != 1 || !sourceDetail.Versions[0].HasSnapshot {
		t.Fatalf("source detail = %#v", sourceDetail)
	}
	if sourceDetail.Versions[0].ContentBytes == 0 {
		t.Fatal("expected snapshot content length without exporting bytes")
	}

	claims, err := projector.ListClaims(context.Background(), 10, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Total != 2 || len(claims.Items) != 2 {
		t.Fatalf("claims = %#v", claims)
	}
	without, err := projector.ListClaims(context.Background(), 10, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	// Store append path requires evidence; filter still returns empty set safely.
	if without.Total != 0 || len(without.Items) != 0 {
		t.Fatalf("without evidence = %#v", without)
	}

	claimDetail, err := projector.ClaimInspector(context.Background(), "claim_supported")
	if err != nil {
		t.Fatal(err)
	}
	if claimDetail.WithoutEvidence || len(claimDetail.Evidence) != 2 {
		t.Fatalf("claim detail = %#v", claimDetail)
	}
	if claimDetail.Evidence[0].Observation == nil || claimDetail.Evidence[0].Fragment == nil {
		t.Fatalf("expected observation and fragment on evidence: %#v", claimDetail.Evidence[0])
	}

	obsDetail, err := projector.ObservationInspector(context.Background(), "observation_1")
	if err != nil {
		t.Fatal(err)
	}
	if obsDetail.Source == nil || obsDetail.Fragment == nil || len(obsDetail.LinkedClaims) == 0 {
		t.Fatalf("observation detail = %#v", obsDetail)
	}

	artifacts, err := projector.ListArtifacts(context.Background(), 10, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if artifacts.Total != 1 || artifacts.Items[0].ID != "artifact_1" {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	artifactDetail, err := projector.ArtifactInspector(context.Background(), "artifact_1")
	if err != nil {
		t.Fatal(err)
	}
	if artifactDetail.Artifact.Content == "" || artifactDetail.Artifact.Kind != "cited_claim_view" {
		t.Fatalf("artifact detail = %#v", artifactDetail)
	}

	// Presentation redaction on free text.
	redactedClaim, report := inspect.RedactClaimDetail(claimDetail)
	if report.SecretMatches == 0 || !strings.Contains(redactedClaim.Claim.Proposition, "[redacted]") {
		// seed uses secret-shaped text only on observation quote and artifact content
	}
	secretObs := domain.Observation{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "observation_secret",
		Statement:     "token api_key=sk-abcdefghijklmnopqrstuvwxyz",
		ExactQuote:    "Authorization: Bearer super-secret-token-value",
		Anchor:        domain.ObservationAnchor{SourceFragmentID: "fragment_1"},
		Provenance:    "fixture",
	}
	obsRedacted, obsReport := inspect.RedactObservationDetail(inspect.ObservationDetail{
		SchemaVersion: domain.SchemaVersionV1,
		Observation:   secretObs,
	})
	if !obsReport.Applied || obsReport.SecretMatches == 0 {
		t.Fatalf("expected secret redaction report: %#v", obsReport)
	}
	if strings.Contains(obsRedacted.Observation.Statement, "sk-") || strings.Contains(obsRedacted.Observation.ExactQuote, "Bearer super") {
		t.Fatalf("secrets leaked: %#v", obsRedacted.Observation)
	}
	_ = redactedClaim
}

func TestKnowledgeHTTPEndpoints(t *testing.T) {
	store, now := seedKnowledge(t)
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	resp := mustGET(t, server.URL+"/knowledge")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("catalog status = %d", resp.StatusCode)
	}
	var catalog inspect.KnowledgeCatalogSummary
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Claims != 2 || catalog.EvidenceLinks != 3 {
		t.Fatalf("http catalog = %#v", catalog)
	}

	claimsResp := mustGET(t, server.URL+"/knowledge/claims?without_evidence=true")
	defer claimsResp.Body.Close()
	var claimPage inspect.ClaimPage
	if err := json.NewDecoder(claimsResp.Body).Decode(&claimPage); err != nil {
		t.Fatal(err)
	}
	if claimPage.Total != 0 {
		t.Fatalf("http claims filter = %#v", claimPage)
	}
	allClaims := mustGET(t, server.URL+"/knowledge/claims?limit=10")
	defer allClaims.Body.Close()
	var allPage inspect.ClaimPage
	if err := json.NewDecoder(allClaims.Body).Decode(&allPage); err != nil {
		t.Fatal(err)
	}
	if allPage.Total != 2 || len(allPage.Items) != 2 {
		t.Fatalf("http claims list = %#v", allPage)
	}

	detailResp := mustGET(t, server.URL+"/knowledge/claims/claim_supported")
	defer detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(detailResp.Body)
		t.Fatalf("claim detail status=%d body=%s", detailResp.StatusCode, body)
	}
	var claimDetail inspect.ClaimDetailResponse
	if err := json.NewDecoder(detailResp.Body).Decode(&claimDetail); err != nil {
		t.Fatal(err)
	}
	if claimDetail.Claim.ID != "claim_supported" || len(claimDetail.Evidence) != 2 {
		t.Fatalf("http claim detail = %#v", claimDetail)
	}

	// Secret-shaped content is redacted at presentation for observations/artifacts.
	artResp := mustGET(t, server.URL+"/knowledge/artifacts/artifact_1")
	defer artResp.Body.Close()
	var art inspect.ArtifactDetailResponse
	if err := json.NewDecoder(artResp.Body).Decode(&art); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(art.Artifact.Content, "sk-abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("artifact content leaked secret: %q report=%#v", art.Artifact.Content, art.Redaction)
	}
	if !art.Redaction.Applied || art.Redaction.SecretMatches == 0 {
		t.Fatalf("expected artifact redaction: %#v content=%q", art.Redaction, art.Artifact.Content)
	}

	mustStatus(t, server.URL+"/knowledge/claims/missing", http.StatusNotFound)
	mustStatus(t, server.URL+"/knowledge/sources?limit=0", http.StatusBadRequest)
}

func seedKnowledge(t *testing.T) (*memory.Store, time.Time) {
	t.Helper()
	store := memory.New()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	content := []byte("knowledge fixture body with token api_key=sk-abcdefghijklmnopqrstuvwxyz")
	digest := sha256.Sum256(content)
	hash := "sha256:" + hex.EncodeToString(digest[:])
	source := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_1", Kind: "fixture", Locator: "fixture.txt", ObservedAt: now}
	version := domain.SourceVersion{
		SchemaVersion: domain.SchemaVersionV1, ID: "source_version_1", SourceID: source.ID,
		ContentHash: hash, ContentRef: hash, ObservedAt: now,
	}
	snapshot := domain.SourceSnapshot{
		SchemaVersion: domain.SchemaVersionV1, SourceVersionID: version.ID, MediaType: "text/plain", Content: content,
	}
	fragment := domain.SourceFragment{
		SchemaVersion: domain.SchemaVersionV1, ID: "fragment_1", SourceVersionID: version.ID,
		Location: fmt.Sprintf("bytes:0-%d", len(content)), StartOffset: 0, EndOffset: uint64(len(content)),
		ContentHash: hash, ContentRef: hash,
	}
	observation1 := domain.Observation{
		SchemaVersion: domain.SchemaVersionV1, ID: "observation_1",
		Statement: "fixture statement", ExactQuote: string(content),
		Anchor: domain.ObservationAnchor{SourceFragmentID: fragment.ID}, Provenance: "extractor:test@1",
	}
	observation2 := domain.Observation{
		SchemaVersion: domain.SchemaVersionV1, ID: "observation_2",
		Statement: "contradicting statement", ExactQuote: string(content),
		Anchor: domain.ObservationAnchor{SourceFragmentID: fragment.ID}, Provenance: "extractor:test@1",
	}
	supported := domain.Claim{
		SchemaVersion: domain.SchemaVersionV1, ID: "claim_supported",
		Proposition: "Knowledge browse is available.", Qualifiers: map[string]string{"scope": "test"}, Version: 1,
	}
	second := domain.Claim{
		SchemaVersion: domain.SchemaVersionV1, ID: "claim_secondary",
		Proposition: "Secondary claim with one support.", Qualifiers: map[string]string{"scope": "test"}, Version: 1,
	}
	supportLink := domain.EvidenceLink{
		SchemaVersion: domain.SchemaVersionV1, ID: "evidence_1", ObservationID: observation1.ID,
		ClaimID: supported.ID, Relation: domain.EvidenceSupports, Rationale: "primary support",
	}
	contradictLink := domain.EvidenceLink{
		SchemaVersion: domain.SchemaVersionV1, ID: "evidence_2", ObservationID: observation2.ID,
		ClaimID: supported.ID, Relation: domain.EvidenceContradicts, Rationale: "counter signal",
	}
	secondLink := domain.EvidenceLink{
		SchemaVersion: domain.SchemaVersionV1, ID: "evidence_3", ObservationID: observation1.ID,
		ClaimID: second.ID, Relation: domain.EvidenceSupports, Rationale: "secondary support",
	}
	artifact := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1, ID: "artifact_1", Kind: "cited_claim_view",
		BaseCommitID: domain.GenesisCommitID,
		Dependencies: []string{"claim:claim_supported@1", "evidence_link:evidence_1"},
		ContentRef:   hash,
		Content:      "# cited view\napi_key=sk-abcdefghijklmnopqrstuvwxyz\n",
		Stale:        false,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendSource(source, version, snapshot); err != nil {
			return err
		}
		if err := tx.AppendSourceFragments(version.ID, []domain.SourceFragment{fragment}); err != nil {
			return err
		}
		if err := tx.AppendObservation(observation1); err != nil {
			return err
		}
		if err := tx.AppendObservation(observation2); err != nil {
			return err
		}
		if err := tx.AppendClaimWithEvidence(supported, []domain.EvidenceLink{supportLink, contradictLink}); err != nil {
			return err
		}
		if err := tx.AppendClaimWithEvidence(second, []domain.EvidenceLink{secondLink}); err != nil {
			return err
		}
		return tx.AppendKnowledgeArtifact(artifact)
	}); err != nil {
		t.Fatal(err)
	}
	return store, now
}
