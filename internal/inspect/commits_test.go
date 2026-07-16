package inspect_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestListCommitsBrowseAndHTTP(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	missionA := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_a", MissionID: "mission_a", Revision: 1,
		OriginalText: "a", Purpose: "a", Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: now,
	}
	missionB := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_b", MissionID: "mission_b", Revision: 1,
		OriginalText: "b", Purpose: "b", Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: now,
	}
	spec := domain.OperationSpec{
		SchemaVersion: domain.SchemaVersionV1, ID: "spec_commits", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "input", OutputSchema: "output", Budget: domain.Budget{Tokens: 100, ModelCalls: 1, Attempts: 1},
		MaxOutputTokens: 20, SafetyMargin: 5, Validators: []string{"schema"}, RetryPolicy: "none",
		FallbackPolicy: "none", MaximumAuthority: domain.AuthorityProposeOnly,
	}

	type seed struct {
		mission domain.MissionRevision
		commit  domain.Commit
	}
	seeds := []seed{
		{missionA, domain.Commit{
			SchemaVersion: domain.SchemaVersionV1, ID: "commit_1", AcceptedChangeSetID: "accepted_1",
			MissionRevision: missionA.ID, BaseCommitID: domain.GenesisCommitID, Version: 1,
			CommittedAt: now, ReceiptID: "commit_receipt_1", IdempotencyKey: "idem_c1",
		}},
		{missionA, domain.Commit{
			SchemaVersion: domain.SchemaVersionV1, ID: "commit_2", AcceptedChangeSetID: "accepted_2",
			MissionRevision: missionA.ID, BaseCommitID: "commit_1", Version: 2,
			CommittedAt: now.Add(time.Minute), ReceiptID: "commit_receipt_2", IdempotencyKey: "idem_c2",
		}},
		{missionB, domain.Commit{
			SchemaVersion: domain.SchemaVersionV1, ID: "commit_b1", AcceptedChangeSetID: "accepted_b1",
			MissionRevision: missionB.ID, BaseCommitID: domain.GenesisCommitID, Version: 1,
			CommittedAt: now.Add(2 * time.Minute), ReceiptID: "commit_receipt_b1", IdempotencyKey: "idem_cb1",
		}},
	}

	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		for _, m := range []domain.MissionRevision{missionA, missionB} {
			if err := tx.AppendMissionRevision(m); err != nil {
				return err
			}
			if err := tx.ActivateMissionRevision(m.MissionID, m.ID); err != nil {
				return err
			}
		}
		for i, item := range seeds {
			n := i + 1
			opID := domain.OperationID(fmt.Sprintf("operation_c%d", n))
			question := domain.Question{
				SchemaVersion: domain.SchemaVersionV1, ID: domain.QuestionID(fmt.Sprintf("question_c%d", n)),
				MissionRevision: item.mission.ID, Text: "q", Origin: "fixture", Relevance: "core", AnswerCondition: "done",
			}
			candidate := domain.InquiryCandidate{
				SchemaVersion: domain.SchemaVersionV1, ID: domain.InquiryCandidateID(fmt.Sprintf("candidate_c%d", n)),
				MissionRevision: item.mission.ID, QuestionID: question.ID, DerivedFrom: []string{"mission"},
				ExpectedProgress: "commit", Novelty: "n", EstimatedCost: domain.Budget{Tokens: 50, ModelCalls: 1, Attempts: 1},
				Risk: domain.RiskLow, SourcePlan: []string{"fixture"}, AnswerCondition: "done", StopCondition: "done",
				ReviewAfter: now.Add(time.Hour),
			}
			inquiry := domain.Inquiry{
				SchemaVersion: domain.SchemaVersionV1, ID: domain.InquiryID(fmt.Sprintf("inquiry_c%d", n)),
				CandidateID: candidate.ID, MissionRevision: item.mission.ID, QuestionID: question.ID,
				AdmissionReason: "seed", Budget: domain.Budget{Tokens: 50, ModelCalls: 1, Attempts: 1}, StopCondition: "done",
				State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
			}
			operation := domain.Operation{
				SchemaVersion: domain.SchemaVersionV1, ID: opID, InquiryID: inquiry.ID, MissionRevision: item.mission.ID,
				SpecID: spec.ID, ReadSet: []string{}, InputRefs: []string{}, ExpectedOutput: "proposed_change_set",
				IdempotencyKey: item.commit.IdempotencyKey, State: domain.StateReady,
				Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
			}
			entityID := fmt.Sprintf("observation_c%d", n)
			proposed := domain.ProposedChangeSet{
				SchemaVersion: domain.SchemaVersionV1, ID: domain.ChangeSetID("proposed_" + string(item.commit.ID)),
				MissionRevision: item.mission.ID, OperationID: opID, BaseCommitID: item.commit.BaseCommitID,
				ReadSet: []string{}, Preconditions: []string{},
				Changes: []domain.Change{{
					Kind: domain.ChangeAdd, EntityType: "observation", EntityID: entityID, PayloadRef: "payload_" + entityID,
				}},
				ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"}, Provenance: "fixture",
				IdempotencyKey: item.commit.IdempotencyKey,
			}
			raw := domain.RawModelOutput{
				SchemaVersion: domain.SchemaVersionV1, ID: domain.ArtifactID(fmt.Sprintf("artifact_c%d", n)),
				OperationID: opID, Model: "fake", Content: `{"ok":true}`,
				ContentHash: fmt.Sprintf("hash_c%d", n), CreatedAt: item.commit.CommittedAt,
			}
			validation := domain.ValidationReceipt{
				SchemaVersion: domain.SchemaVersionV1, ID: domain.ReceiptID(fmt.Sprintf("validation_c%d", n)),
				OperationID: opID, ChangeSetID: proposed.ID, ValidatorID: "schema", Passed: true,
				ArtifactRef: raw.ID, ProducedAt: item.commit.CommittedAt,
			}
			accepted := domain.AcceptedChangeSet{
				SchemaVersion: domain.SchemaVersionV1, ID: item.commit.AcceptedChangeSetID,
				ProposedChangeSetID: proposed.ID, ValidationReceiptIDs: []domain.ReceiptID{validation.ID},
				AcceptedAt: item.commit.CommittedAt, PolicyVersion: "v1",
			}
			receipt := domain.CommitReceipt{
				SchemaVersion: domain.SchemaVersionV1, ID: item.commit.ReceiptID, CommitID: item.commit.ID,
				ChangeSetID: accepted.ID, OperationID: opID, Version: item.commit.Version,
				ProducedAt: item.commit.CommittedAt,
			}
			if err := tx.CreateQuestion(question); err != nil {
				return err
			}
			if err := tx.CreateInquiryCandidate(candidate); err != nil {
				return err
			}
			if err := tx.CreateInquiry(inquiry); err != nil {
				return err
			}
			if err := tx.CreateOperation(operation); err != nil {
				return err
			}
			if err := tx.AppendRawModelOutput(raw); err != nil {
				return err
			}
			if err := tx.AppendProposedChangeSet(proposed); err != nil {
				return err
			}
			if err := tx.AppendValidationReceipt(validation); err != nil {
				return err
			}
			if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
				return err
			}
			if err := tx.ApplyCommit(item.commit, receipt, proposed.Changes); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	all, err := projector.ListCommits(context.Background(), 50, 0, inspect.CommitFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 3 || len(all.Items) != 3 {
		t.Fatalf("all commits = %#v", all)
	}
	// Stable Version then ID: commit_1, commit_b1 (v1), then commit_2 (v2).
	if all.Items[0].ID != "commit_1" || all.Items[1].ID != "commit_b1" || all.Items[2].ID != "commit_2" {
		t.Fatalf("order = %#v", all.Items)
	}
	if all.Items[0].IsHead || !all.Items[1].IsHead || !all.Items[2].IsHead {
		t.Fatalf("head flags = %#v", all.Items)
	}

	heads, err := projector.ListCommits(context.Background(), 50, 0, inspect.CommitFilter{HeadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if heads.Total != 2 || !heads.HeadOnly {
		t.Fatalf("heads = %#v", heads)
	}

	revA, err := projector.ListCommits(context.Background(), 50, 0, inspect.CommitFilter{MissionRevision: missionA.ID})
	if err != nil {
		t.Fatal(err)
	}
	if revA.Total != 2 || revA.MissionRevision != missionA.ID {
		t.Fatalf("rev filter = %#v", revA)
	}

	page, err := projector.ListCommits(context.Background(), 1, 1, inspect.CommitFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 1 || page.Items[0].ID != "commit_b1" || page.Offset != 1 {
		t.Fatalf("pagination = %#v", page)
	}

	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	resp := mustGET(t, server.URL+"/commits?limit=10")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var httpPage inspect.CommitPage
	if err := json.NewDecoder(resp.Body).Decode(&httpPage); err != nil {
		t.Fatal(err)
	}
	if httpPage.Total != 3 || len(httpPage.Items) != 3 {
		t.Fatalf("http page = %#v", httpPage)
	}

	headResp := mustGET(t, server.URL+"/commits?head_only=true&mission_revision_id=revision_a")
	defer headResp.Body.Close()
	var headPage inspect.CommitPage
	if err := json.NewDecoder(headResp.Body).Decode(&headPage); err != nil {
		t.Fatal(err)
	}
	if headPage.Total != 1 || len(headPage.Items) != 1 || headPage.Items[0].ID != "commit_2" {
		t.Fatalf("http head filter = %#v", headPage)
	}
	if !headPage.HeadOnly || headPage.MissionRevision != missionA.ID {
		t.Fatalf("http head echo = %#v", headPage)
	}

	bad := mustGET(t, server.URL+"/commits?head_only=maybe")
	defer bad.Body.Close()
	if bad.StatusCode != 400 {
		t.Fatalf("invalid head_only status = %d", bad.StatusCode)
	}
}
