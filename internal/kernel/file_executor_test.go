package kernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func fileDiscoverTestSpec() domain.OperationSpec {
	return domain.OperationSpec{
		SchemaVersion: 1, ID: "file.discover@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "file.discover.input.v1", OutputSchema: "file.discover.output.v1",
		Budget:          domain.Budget{Attempts: 3, Tokens: 100, Bytes: 64 << 10},
		MaxOutputTokens: 50, SafetyMargin: 10, Validators: []string{"schema"},
		RetryPolicy: "no_retry", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityReadOnly,
	}
}

func fileReadTestSpec() domain.OperationSpec {
	return domain.OperationSpec{
		SchemaVersion: 1, ID: "file.read@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "file.read.input.v1", OutputSchema: "file.read.output.v1",
		Budget:          domain.Budget{Attempts: 3, Tokens: 100, Bytes: 64 << 10},
		MaxOutputTokens: 50, SafetyMargin: 10, Validators: []string{"schema"},
		RetryPolicy: "no_retry", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityReadOnly,
	}
}

func seedFileAgenda(t *testing.T, store port.Store, now time.Time, discover bool, inputRefs []string) {
	t.Helper()
	err := store.Update(context.Background(), func(tx port.Transaction) error {
		revision := domain.MissionRevision{
			SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
			OriginalText: "inspect local files", Purpose: "knowledge", Domains: []string{"ops"},
			Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "user",
			AcceptedAt: now, Budget: domain.Budget{Attempts: 10, Tokens: 8000, Bytes: 1 << 20},
		}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		var spec domain.OperationSpec
		if discover {
			spec = fileDiscoverTestSpec()
		} else {
			spec = fileReadTestSpec()
		}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{
			SchemaVersion: 1, ID: "question_1", MissionRevision: revision.ID,
			Text: "what files exist?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence",
		}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{
			SchemaVersion: 1, ID: "candidate_1", MissionRevision: revision.ID, QuestionID: question.ID,
			DerivedFrom: []string{"gap_1"}, ExpectedProgress: "reduce uncertainty", Novelty: "new",
			Risk: domain.RiskLow, SourcePlan: []string{"file"}, AnswerCondition: "evidence",
			StopCondition: "done", ReviewAfter: now.Add(time.Hour),
		}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{
			SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: revision.ID,
			QuestionID: question.ID, AdmissionReason: "priority", StopCondition: "done",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		op := domain.Operation{
			SchemaVersion: 1, ID: "operation_file", InquiryID: inquiry.ID, MissionRevision: revision.ID,
			SpecID: spec.ID, ReadSet: []string{}, ExpectedOutput: "file_result",
			IdempotencyKey: "idem_file", InputRefs: inputRefs,
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		return tx.CreateOperation(op)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFileEligible(t *testing.T) {
	t.Parallel()
	discover := fileDiscoverTestSpec()
	if !FileEligible(discover) {
		t.Fatal("file.discover should be file-eligible")
	}
	if LocalEligible(discover) {
		t.Fatal("file.discover must not be local-eligible")
	}
	if ModelEligible(discover) {
		t.Fatal("file.discover must not be model-eligible")
	}
	if WebEligible(discover) {
		t.Fatal("file.discover must not be web-eligible")
	}
	read := fileReadTestSpec()
	if !FileEligible(read) {
		t.Fatal("file.read should be file-eligible")
	}
	local := ContinuityOperationSpec("continuity.integrity_audit@1", domain.AuthorityReadOnly)
	if FileEligible(local) {
		t.Fatal("continuity must not be file-eligible")
	}
}

func TestFileExecutorDiscoverAndRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	store := memory.New()
	seedFileAgenda(t, store, now, true, []string{"root:default", "path:.", "pattern:*.txt"})
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@file-test")
	if err != nil {
		t.Fatal(err)
	}
	exec := FileExecutor{
		Store: store, Clock: clock, IDs: ids, Authorizer: auth,
		Roots: []FileRoot{{Name: "default", Path: root}},
	}
	result, err := exec.Execute(ctx, "operation_file")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !result.Completed || result.EntryCount != 1 {
		t.Fatalf("discover result = %#v", result)
	}

	// Second op: read the file.
	store2 := memory.New()
	seedFileAgenda(t, store2, now, false, []string{"root:default", "path:notes.txt"})
	auth2, err := NewMVPCapabilityAuthorizer(store2, clock, "policy@file-test")
	if err != nil {
		t.Fatal(err)
	}
	exec2 := FileExecutor{
		Store: store2, Clock: clock, IDs: ids, Authorizer: auth2,
		Roots: []FileRoot{{Name: "default", Path: root}},
	}
	readResult, err := exec2.Execute(ctx, "operation_file")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !readResult.Completed || readResult.BytesRead != len("hello fixture") {
		t.Fatalf("read result = %#v", readResult)
	}
	var art domain.KnowledgeArtifact
	if err := store2.View(ctx, func(r port.Reader) error {
		var err error
		art, err = r.KnowledgeArtifact(readResult.ArtifactID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(art.Content), &body); err != nil {
		t.Fatal(err)
	}
	if body["content"] != "hello fixture" {
		t.Fatalf("artifact content = %v", body["content"])
	}
	if body["trust"] != "untrusted_source_data" {
		t.Fatalf("trust marker missing: %v", body["trust"])
	}
}

func TestFileExecutorRejectsTraversal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)

	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Symlink pointing outside the authorized root.
	if err := os.Symlink(secret, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}

	store := memory.New()
	seedFileAgenda(t, store, now, false, []string{"root:default", "path:../" + filepath.Base(outside) + "/secret.txt"})
	exec := FileExecutor{
		Store: store, Clock: clock, IDs: ids,
		Roots: []FileRoot{{Name: "default", Path: root}},
	}
	result, err := exec.Execute(ctx, "operation_file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "path_not_authorized" {
		t.Fatalf("expected path_not_authorized skip, got %#v", result)
	}

	// Symlink escape must not succeed either.
	store2 := memory.New()
	seedFileAgenda(t, store2, now, false, []string{"root:default", "path:escape.txt"})
	exec2 := FileExecutor{
		Store: store2, Clock: clock, IDs: ids,
		Roots: []FileRoot{{Name: "default", Path: root}},
	}
	result2, err := exec2.Execute(ctx, "operation_file")
	if err == nil && result2.Completed {
		t.Fatalf("symlink escape must not complete: %#v", result2)
	}
	if err == nil && result2.Skipped && result2.SkipReason != "path_not_authorized" {
		// failRunning path also acceptable (effect error → replan)
		if !strings.Contains(result2.SkipReason, "path") && result2.SkipReason != "" {
			// completed=false without skip means fail/replan returned error
		}
	}
	if result2.Completed {
		t.Fatalf("symlink escape completed: %#v", result2)
	}
}

func TestFileExecutorOversizeRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Repeat("x", 64)), 0o644); err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	seedFileAgenda(t, store, now, false, []string{"root:default", "path:big.txt"})
	exec := FileExecutor{
		Store: store, Clock: clock, IDs: ids,
		Roots:        []FileRoot{{Name: "default", Path: root}},
		MaxReadBytes: 16,
	}
	_, err := exec.Execute(ctx, "operation_file")
	if err == nil {
		t.Fatal("expected oversize read error")
	}
	if !strings.Contains(err.Error(), "max read bytes") {
		t.Fatalf("unexpected error: %v", err)
	}
	// Operation should be replanned back to READY (not SUCCEEDED).
	var op domain.Operation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_file")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateReady {
		t.Fatalf("state after oversize = %s", op.State)
	}
}

func TestDispatchRequiresFileWhenUnwired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedFileAgenda(t, store, now, true, []string{"root:default", "path:."})
	dispatch := DispatchExecutor{
		Store: store,
		Local: LocalExecutor{Store: store, Clock: clock, IDs: ids},
	}
	result, err := dispatch.Execute(ctx, "operation_file")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || result.SkipReason != "requires_file" {
		t.Fatalf("expected requires_file, got %#v", result)
	}
}

func TestFileReserveThrottlesOnZeroBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	root := t.TempDir()
	store := memory.New()
	// Seed with zero budget attempts.
	err := store.Update(ctx, func(tx port.Transaction) error {
		revision := domain.MissionRevision{
			SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
			OriginalText: "inspect", Purpose: "knowledge", Domains: []string{"ops"},
			Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "user",
			AcceptedAt: now, Budget: domain.Budget{Attempts: 10, Tokens: 8000, Bytes: 1 << 20},
		}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		spec := fileDiscoverTestSpec()
		// Keep schema valid; zero Attempts is fail-closed for FR-RES-001.
		spec.Budget = domain.Budget{Attempts: 0, Tokens: 100, Bytes: 64 << 10}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		if err := tx.CreateQuestion(domain.Question{
			SchemaVersion: 1, ID: "question_1", MissionRevision: revision.ID,
			Text: "q", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence",
		}); err != nil {
			return err
		}
		if err := tx.CreateInquiryCandidate(domain.InquiryCandidate{
			SchemaVersion: 1, ID: "candidate_1", MissionRevision: revision.ID, QuestionID: "question_1",
			DerivedFrom: []string{"g"}, ExpectedProgress: "p", Novelty: "n", Risk: domain.RiskLow,
			SourcePlan: []string{"file"}, AnswerCondition: "evidence", StopCondition: "done",
			ReviewAfter: now.Add(time.Hour),
		}); err != nil {
			return err
		}
		if err := tx.CreateInquiry(domain.Inquiry{
			SchemaVersion: 1, ID: "inquiry_1", CandidateID: "candidate_1", MissionRevision: revision.ID,
			QuestionID: "question_1", AdmissionReason: "priority", StopCondition: "done",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}); err != nil {
			return err
		}
		return tx.CreateOperation(domain.Operation{
			SchemaVersion: 1, ID: "operation_file", InquiryID: "inquiry_1", MissionRevision: revision.ID,
			SpecID: spec.ID, ExpectedOutput: "file_result", IdempotencyKey: "idem_zero",
			InputRefs: []string{"root:default", "path:."},
			State:     domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@file-test")
	if err != nil {
		t.Fatal(err)
	}
	exec := FileExecutor{
		Store: store, Clock: clock, IDs: ids, Authorizer: auth,
		Roots: []FileRoot{{Name: "default", Path: root}},
	}
	result, err := exec.Execute(ctx, "operation_file")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Fatalf("zero budget must skip, got %#v", result)
	}
}
