package kernel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// FamilySpecCatalog maps continuity families to the OperationSpec that may be
// instantiated when an opportunity is admitted. Specs remain kernel-authored;
// model output never selects authority or validators.
type FamilySpecCatalog map[domain.WorkFamily]domain.OperationSpecID

// DefaultFamilySpecCatalog returns the MVP continuity catalogue ids. Tests and
// bootstraps must AppendOperationSpec for each id before admission.
func DefaultFamilySpecCatalog() FamilySpecCatalog {
	return FamilySpecCatalog{
		domain.FamilyGapScan:           "continuity.gap_scan@1",
		domain.FamilyConflictReview:    "continuity.conflict_review@1",
		domain.FamilyArtifactRefresh:   "continuity.artifact_refresh@1",
		domain.FamilyIntegrityAudit:    "continuity.integrity_audit@1",
		domain.FamilyHarnessEvaluation: "continuity.harness_evaluation@1",
		domain.FamilyFrontierManage:    "continuity.frontier_manage@1",
		domain.FamilyCoverageScan:      "continuity.coverage_scan@1",
		domain.FamilySourceFreshness:   "continuity.source_freshness@1",
	}
}

// ContinuityOperationSpec builds a conservative OperationSpec for a family.
// Callers persist it via AppendOperationSpec; the helper only constructs values.
func ContinuityOperationSpec(id domain.OperationSpecID, authority domain.Authority) domain.OperationSpec {
	if authority == "" {
		authority = domain.AuthorityProposeOnly
	}
	return domain.OperationSpec{
		SchemaVersion:    domain.SchemaVersionV1,
		ID:               id,
		ContractVersion:  1,
		TemplateVersion:  1,
		InputSchema:      "work-opportunity-v1",
		OutputSchema:     "proposed-changeset-or-diagnosis-v1",
		Budget:           domain.Budget{ModelCalls: 1, Tokens: 2048, Attempts: 1, Duration: 15 * time.Minute},
		MaxOutputTokens:  256,
		SafetyMargin:     128,
		Validators:       []string{"schema", "authority"},
		RetryPolicy:      "none",
		FallbackPolicy:   "another-continuity-strategy",
		MaximumAuthority: authority,
	}
}

// Admitter materialises open WorkOpportunities into recoverable agenda units.
// Admission is deterministic and bound by HorizonPolicy marks.
type Admitter struct {
	Store   port.Store
	Clock   source.Clock
	IDs     source.IDGenerator
	Policy  domain.HorizonPolicy
	Catalog FamilySpecCatalog
}

func (a Admitter) policy() domain.HorizonPolicy {
	if a.Policy.Version == "" && a.Policy.SchemaVersion == 0 {
		return domain.DefaultHorizonPolicy()
	}
	return a.Policy
}

func (a Admitter) catalog() FamilySpecCatalog {
	if len(a.Catalog) == 0 {
		return DefaultFamilySpecCatalog()
	}
	return a.Catalog
}

// AdmitResult is the durable effect of admitting a single opportunity.
type AdmitResult struct {
	Opportunity domain.WorkOpportunity
	Question    domain.Question
	Candidate   domain.InquiryCandidate
	Inquiry     domain.Inquiry
	Operation   domain.Operation
}

// AdmitOne converts one OPEN opportunity into Question→Inquiry→Operation and
// marks the opportunity ADMITTED in the same transaction.
func (a Admitter) AdmitOne(ctx context.Context, opportunityID domain.WorkOpportunityID) (AdmitResult, error) {
	if err := a.validateDeps(); err != nil {
		return AdmitResult{}, err
	}
	policy := a.policy()
	if err := policy.Validate(); err != nil {
		return AdmitResult{}, fmt.Errorf("horizon policy: %w", err)
	}

	var result AdmitResult
	err := a.Store.Update(ctx, func(tx port.Transaction) error {
		opportunity, err := tx.WorkOpportunity(opportunityID)
		if err != nil {
			return err
		}
		if opportunity.Status != domain.OpportunityOpen {
			return fmt.Errorf("%w: work opportunity %s is %s, want OPEN", port.ErrConflict, opportunity.ID, opportunity.Status)
		}
		ready, err := countReady(tx, opportunity.MissionRevision)
		if err != nil {
			return err
		}
		if !policy.AcceptsAdmission(ready) {
			return fmt.Errorf("%w: ready horizon at max_ready=%d", port.ErrConflict, policy.MaxReady)
		}
		built, err := a.buildAdmission(tx, opportunity, policy)
		if err != nil {
			return err
		}
		if err := persistAdmission(tx, built, a.Clock.Now().UTC()); err != nil {
			return err
		}
		result = built
		return nil
	})
	if err != nil {
		return AdmitResult{}, err
	}
	return result, nil
}

// AdmitFromFrontier admits open opportunities in priority order until the
// target ready count is reached, max_ready is hit, or the open set is exhausted.
// Deferred opportunities are never auto-admitted here.
func (a Admitter) AdmitFromFrontier(ctx context.Context, mission domain.MissionRevisionID) (ContinuityResult, error) {
	if err := a.validateDeps(); err != nil {
		return ContinuityResult{}, err
	}
	if mission == "" {
		return ContinuityResult{}, errors.New("mission revision is required")
	}
	policy := a.policy()
	if err := policy.Validate(); err != nil {
		return ContinuityResult{}, fmt.Errorf("horizon policy: %w", err)
	}

	var result ContinuityResult
	// Loop with short transactions so each admission is independently durable.
	for {
		var admitted bool
		err := a.Store.Update(ctx, func(tx port.Transaction) error {
			ready, err := countReady(tx, mission)
			if err != nil {
				return err
			}
			if ready >= policy.TargetReady || !policy.AcceptsAdmission(ready) {
				return nil
			}
			open, err := tx.WorkOpportunities(mission, domain.OpportunityOpen)
			if err != nil {
				return err
			}
			if len(open) == 0 {
				return nil
			}
			// open is already priority-desc, created_at asc, id asc.
			candidate := open[0]
			built, err := a.buildAdmission(tx, candidate, policy)
			if err != nil {
				return err
			}
			if err := persistAdmission(tx, built, a.Clock.Now().UTC()); err != nil {
				return err
			}
			admitted = true
			return nil
		})
		if err != nil {
			return result, err
		}
		if !admitted {
			break
		}
		result.Admitted++
		result.Changed = true
	}
	return result, nil
}

func (a Admitter) validateDeps() error {
	if a.Store == nil || a.Clock == nil || a.IDs == nil {
		return errors.New("admitter dependencies are incomplete")
	}
	return nil
}

func (a Admitter) buildAdmission(tx port.Transaction, opportunity domain.WorkOpportunity, policy domain.HorizonPolicy) (AdmitResult, error) {
	if err := opportunity.Validate(); err != nil {
		return AdmitResult{}, err
	}
	if opportunity.Status != domain.OpportunityOpen {
		return AdmitResult{}, fmt.Errorf("%w: only OPEN opportunities may be admitted", port.ErrConflict)
	}
	if _, err := tx.MissionRevision(opportunity.MissionRevision); err != nil {
		return AdmitResult{}, err
	}
	specID, ok := a.catalog()[opportunity.Family]
	if !ok || specID == "" {
		return AdmitResult{}, fmt.Errorf("no operation spec registered for family %q", opportunity.Family)
	}
	if _, err := tx.OperationSpec(specID); err != nil {
		return AdmitResult{}, fmt.Errorf("operation spec %s for family %s: %w", specID, opportunity.Family, err)
	}

	questionID, err := a.newID("question")
	if err != nil {
		return AdmitResult{}, err
	}
	candidateID, err := a.newID("inquiry_candidate")
	if err != nil {
		return AdmitResult{}, err
	}
	inquiryID, err := a.newID("inquiry")
	if err != nil {
		return AdmitResult{}, err
	}
	operationID, err := a.newID("operation")
	if err != nil {
		return AdmitResult{}, err
	}
	idempotencyKey, err := a.newID("idempotency")
	if err != nil {
		return AdmitResult{}, err
	}

	now := a.Clock.Now().UTC()
	if now.Before(opportunity.CreatedAt) {
		// Fixtures may stamp CreatedAt after a frozen test clock.
		now = opportunity.CreatedAt.UTC()
	}
	if !opportunity.UpdatedAt.IsZero() && now.Before(opportunity.UpdatedAt) {
		now = opportunity.UpdatedAt.UTC()
	}
	reviewAfter := now.Add(24 * time.Hour)
	sourcePlan := []string{"work_opportunity:" + string(opportunity.ID), "family:" + string(opportunity.Family)}
	if len(opportunity.Dependencies) > 0 {
		sourcePlan = append(sourcePlan, opportunity.Dependencies...)
	}
	derivedFrom := []string{"work_opportunity:" + string(opportunity.ID)}
	if opportunity.ParentID != "" {
		derivedFrom = append(derivedFrom, "parent_opportunity:"+string(opportunity.ParentID))
	}
	budget := opportunity.EstimatedCost
	if budget == (domain.Budget{}) {
		budget = domain.Budget{Tokens: 256, Attempts: 1}
	}
	inquiryBudget := budget
	if inquiryBudget.Attempts == 0 {
		inquiryBudget.Attempts = 1
	}

	question := domain.Question{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              domain.QuestionID(questionID),
		MissionRevision: opportunity.MissionRevision,
		Text:            opportunity.Title,
		Origin:          opportunity.Origin,
		Relevance:       "continuity:" + string(opportunity.Family),
		AnswerCondition: opportunity.StopCondition,
	}
	candidate := domain.InquiryCandidate{
		SchemaVersion:    domain.SchemaVersionV1,
		ID:               domain.InquiryCandidateID(candidateID),
		MissionRevision:  opportunity.MissionRevision,
		QuestionID:       question.ID,
		DerivedFrom:      derivedFrom,
		ExpectedProgress: opportunity.ExpectedGain,
		Novelty:          opportunity.Novelty,
		EstimatedCost:    budget,
		Risk:             opportunity.Risk,
		SourcePlan:       sourcePlan,
		AnswerCondition:  opportunity.StopCondition,
		StopCondition:    opportunity.StopCondition,
		ReviewAfter:      reviewAfter,
	}
	inquiry := domain.Inquiry{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              domain.InquiryID(inquiryID),
		CandidateID:     candidate.ID,
		MissionRevision: opportunity.MissionRevision,
		QuestionID:      question.ID,
		AdmissionReason: fmt.Sprintf("admitted from work opportunity %s under policy %s", opportunity.ID, policy.Version),
		Budget:          inquiryBudget,
		StopCondition:   opportunity.StopCondition,
		State:           domain.StateReady,
		Reevaluation:    domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	operation := domain.Operation{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              domain.OperationID(operationID),
		InquiryID:       inquiry.ID,
		MissionRevision: opportunity.MissionRevision,
		SpecID:          specID,
		ReadSet:         []string{"work_opportunity:" + string(opportunity.ID)},
		InputRefs:       []string{string(opportunity.ID), opportunity.DedupSignature},
		ExpectedOutput:  "proposed change set, diagnosis, or verified delta",
		IdempotencyKey:  domain.IdempotencyKey(idempotencyKey),
		State:           domain.StateReady,
		Reevaluation:    domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	if err := question.Validate(); err != nil {
		return AdmitResult{}, fmt.Errorf("build question: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return AdmitResult{}, fmt.Errorf("build candidate: %w", err)
	}
	if err := inquiry.Validate(); err != nil {
		return AdmitResult{}, fmt.Errorf("build inquiry: %w", err)
	}
	if err := operation.Validate(); err != nil {
		return AdmitResult{}, fmt.Errorf("build operation: %w", err)
	}

	admitted := opportunity
	admitted.Status = domain.OpportunityAdmitted
	admitted.AdmittedInquiryID = inquiry.ID
	admitted.UpdatedAt = now
	if admitted.UpdatedAt.Before(admitted.CreatedAt) {
		// Admission clock may lag a fixture CreatedAt; never move UpdatedAt backwards.
		admitted.UpdatedAt = admitted.CreatedAt
	}
	if err := admitted.Validate(); err != nil {
		return AdmitResult{}, fmt.Errorf("mark admitted: %w", err)
	}

	return AdmitResult{
		Opportunity: admitted,
		Question:    question,
		Candidate:   candidate,
		Inquiry:     inquiry,
		Operation:   operation,
	}, nil
}

func persistAdmission(tx port.Transaction, built AdmitResult, now time.Time) error {
	if err := tx.CreateQuestion(built.Question); err != nil {
		return err
	}
	if err := tx.CreateInquiryCandidate(built.Candidate); err != nil {
		return err
	}
	if err := tx.CreateInquiry(built.Inquiry); err != nil {
		return err
	}
	if err := tx.CreateOperation(built.Operation); err != nil {
		return err
	}
	if err := tx.SaveWorkOpportunity(built.Opportunity); err != nil {
		return err
	}
	eventID := domain.EventID(string(built.Opportunity.ID) + ":admitted")
	_, err := tx.AppendEvent(domain.Event{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              eventID,
		Kind:            domain.EventContinuityExpanded,
		OccurredAt:      now,
		MissionRevision: built.Opportunity.MissionRevision,
		InquiryID:       built.Inquiry.ID,
		OperationID:     built.Operation.ID,
		PayloadRef:      string(built.Opportunity.ID),
	})
	return err
}

func (a Admitter) newID(prefix string) (string, error) {
	id, err := a.IDs.NewID(prefix)
	if err != nil {
		return "", fmt.Errorf("generate %s ID: %w", prefix, err)
	}
	if strings.TrimSpace(id) == "" {
		return "", errors.New("generated ID must not be empty")
	}
	return id, nil
}

func countReady(tx port.Reader, mission domain.MissionRevisionID) (int, error) {
	operations, err := tx.Operations(mission)
	if err != nil {
		return 0, err
	}
	ready := 0
	for _, operation := range operations {
		if operation.State == domain.StateReady {
			ready++
		}
	}
	return ready, nil
}
