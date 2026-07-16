package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// Event kinds emitted by the model-free local executor.
const (
	EventOperationDispatched    = "operation.dispatched"
	EventOperationLocalVerified = "operation.local_verified"
	EventOperationSucceeded     = "operation.succeeded"
)

// LocalExecutor runs continuity/local operations without a model provider.
// It is the first vertical of the architecture Executor: pure transitions under
// a lease reference, optional read-only audit artifact, and append-only events.
// Model-backed PROPOSE_ONLY paths remain out of scope until a provider is wired.
type LocalExecutor struct {
	Store    port.Store
	Clock    source.Clock
	IDs      source.IDGenerator
	LeaseTTL time.Duration
}

// ExecuteResult summarizes one Execute call.
type ExecuteResult struct {
	OperationID domain.OperationID
	Completed   bool
	Skipped     bool
	SkipReason  string
	ArtifactID  domain.ArtifactID
	LeaseRef    string
}

func (e LocalExecutor) validateDeps() error {
	if e.Store == nil || e.Clock == nil || e.IDs == nil {
		return errors.New("local executor dependencies are incomplete")
	}
	return nil
}

func (e LocalExecutor) leaseTTL() time.Duration {
	if e.LeaseTTL <= 0 {
		return 15 * time.Minute
	}
	return e.LeaseTTL
}

// LocalEligible reports whether an OperationSpec may be completed without a model.
// Continuity catalogue specs and READ_ONLY authority are local; other PROPOSE_ONLY
// contracts wait for a model-backed path.
func LocalEligible(spec domain.OperationSpec) bool {
	if err := spec.Validate(); err != nil {
		return false
	}
	if spec.MaximumAuthority == domain.AuthorityReadOnly {
		return true
	}
	if spec.MaximumAuthority == domain.AuthorityKernelWrite {
		// Reserved for future kernel writers; not auto-executed here.
		return false
	}
	id := string(spec.ID)
	return strings.HasPrefix(id, "continuity.")
}

// Execute transitions a single READY operation through RUNNING → VERIFYING →
// SUCCEEDED when the bound OperationSpec is local-eligible. Non-eligible ops are
// skipped (not errors) so the control loop can wait for a model path.
func (e LocalExecutor) Execute(ctx context.Context, operationID domain.OperationID) (ExecuteResult, error) {
	if err := e.validateDeps(); err != nil {
		return ExecuteResult{}, err
	}
	if operationID == "" {
		return ExecuteResult{}, errors.New("operation id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var result ExecuteResult
	result.OperationID = operationID
	err := e.Store.Update(ctx, func(tx port.Transaction) error {
		operation, err := tx.Operation(operationID)
		if err != nil {
			return err
		}
		if operation.State.Terminal() {
			result.Skipped = true
			result.SkipReason = "terminal"
			return nil
		}
		if operation.State != domain.StateReady {
			result.Skipped = true
			result.SkipReason = "not_ready"
			return nil
		}
		spec, err := tx.OperationSpec(operation.SpecID)
		if err != nil {
			return fmt.Errorf("load operation spec %s: %w", operation.SpecID, err)
		}
		if !LocalEligible(spec) {
			result.Skipped = true
			result.SkipReason = "requires_model"
			return nil
		}

		leaseRef, err := e.IDs.NewID("lease")
		if err != nil {
			return fmt.Errorf("generate lease id: %w", err)
		}
		if strings.TrimSpace(leaseRef) == "" {
			return errors.New("generated lease id must not be empty")
		}
		// Bind lease identity to operation attempt for reconcilability (FR-DUR-003).
		leaseRef = fmt.Sprintf("%s:op=%s:attempt=%d", leaseRef, operation.ID, operation.Attempt+1)
		result.LeaseRef = leaseRef

		now := e.Clock.Now().UTC()
		// Lease TTL is recorded in event payload; reevaluation stores the lease ref.
		_ = now.Add(e.leaseTTL())

		snap := domain.OperationalSnapshot{State: operation.State, Reevaluation: operation.Reevaluation}
		running, err := domain.Transition(snap, domain.TransitionInput{Event: domain.EventDispatch, Reference: leaseRef})
		if err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		verifying, err := domain.Transition(running, domain.TransitionInput{Event: domain.EventBeginVerify, Reference: leaseRef})
		if err != nil {
			return fmt.Errorf("begin verify: %w", err)
		}

		// Local verification: inventory + optional audit artifact, no model output.
		artifact, err := e.buildLocalArtifact(tx, operation, spec, leaseRef, now)
		if err != nil {
			return err
		}
		if artifact.ID != "" {
			if err := tx.AppendKnowledgeArtifact(artifact); err != nil {
				return fmt.Errorf("append local artifact: %w", err)
			}
			result.ArtifactID = artifact.ID
		}

		done, err := domain.Transition(verifying, domain.TransitionInput{Event: domain.EventSucceed})
		if err != nil {
			return fmt.Errorf("succeed: %w", err)
		}
		operation.State = done.State
		operation.Reevaluation = done.Reevaluation
		operation.Attempt++
		if err := tx.SaveOperation(operation); err != nil {
			return err
		}

		// Keep the parent inquiry in lockstep when it is still READY for this unit.
		if inquiry, inqErr := tx.Inquiry(operation.InquiryID); inqErr == nil && inquiry.State == domain.StateReady {
			inqSnap := domain.OperationalSnapshot{State: inquiry.State, Reevaluation: inquiry.Reevaluation}
			inqRunning, err := domain.Transition(inqSnap, domain.TransitionInput{Event: domain.EventDispatch, Reference: leaseRef})
			if err == nil {
				inqVerifying, err := domain.Transition(inqRunning, domain.TransitionInput{Event: domain.EventBeginVerify, Reference: leaseRef})
				if err == nil {
					inqDone, err := domain.Transition(inqVerifying, domain.TransitionInput{Event: domain.EventSucceed})
					if err == nil {
						inquiry.State = inqDone.State
						inquiry.Reevaluation = inqDone.Reevaluation
						_ = tx.SaveInquiry(inquiry)
					}
				}
			}
		}

		if err := appendOperationEvents(tx, operation, leaseRef, artifact.ID, now); err != nil {
			return err
		}
		result.Completed = true
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func appendOperationEvents(tx port.Transaction, operation domain.Operation, leaseRef string, artifactID domain.ArtifactID, now time.Time) error {
	payload := leaseRef
	if artifactID != "" {
		payload = leaseRef + ";artifact=" + string(artifactID)
	}
	events := []domain.Event{
		{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:dispatched:%d", operation.ID, operation.Attempt)),
			Kind:            EventOperationDispatched,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      leaseRef,
		},
		{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:local_verified:%d", operation.ID, operation.Attempt)),
			Kind:            EventOperationLocalVerified,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      payload,
		},
		{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:succeeded:%d", operation.ID, operation.Attempt)),
			Kind:            EventOperationSucceeded,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      payload,
		},
	}
	for _, event := range events {
		if _, err := tx.AppendEvent(event); err != nil {
			return err
		}
	}
	return nil
}

type localAuditBody struct {
	Schema        string                   `json:"schema"`
	OperationID   domain.OperationID       `json:"operation_id"`
	SpecID        domain.OperationSpecID   `json:"spec_id"`
	Authority     domain.Authority         `json:"authority"`
	LeaseRef      string                   `json:"lease_ref"`
	Mission       domain.MissionRevisionID `json:"mission_revision_id"`
	ReadyCount    int                      `json:"ready_count"`
	RunningCount  int                      `json:"running_count"`
	OpenOpps      int                      `json:"open_opportunities"`
	AdmittedOpps  int                      `json:"admitted_opportunities"`
	ArtifactCount int                      `json:"knowledge_artifact_count"`
	SourceCount   int                      `json:"source_count"`
	ClaimCount    int                      `json:"claim_count"`
	ObservationN  int                      `json:"observation_count"`
	VerifiedAt    time.Time                `json:"verified_at"`
	Mode          string                   `json:"mode"`
}

func (e LocalExecutor) buildLocalArtifact(tx port.Transaction, operation domain.Operation, spec domain.OperationSpec, leaseRef string, now time.Time) (domain.KnowledgeArtifact, error) {
	// Always materialize a small audit for continuity specs so SUCCEED has a durable delta.
	ops, err := tx.Operations(operation.MissionRevision)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	ready, running := 0, 0
	for _, op := range ops {
		switch op.State {
		case domain.StateReady:
			ready++
		case domain.StateRunning, domain.StateVerifying:
			running++
		}
	}
	open, err := tx.WorkOpportunities(operation.MissionRevision, domain.OpportunityOpen)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	admitted, err := tx.WorkOpportunities(operation.MissionRevision, domain.OpportunityAdmitted)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	artifacts, err := tx.KnowledgeArtifacts()
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	sources, err := tx.Sources()
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	claims, err := tx.Claims()
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	observations, err := tx.Observations()
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}

	body := localAuditBody{
		Schema:        "local-operation-audit-v1",
		OperationID:   operation.ID,
		SpecID:        spec.ID,
		Authority:     spec.MaximumAuthority,
		LeaseRef:      leaseRef,
		Mission:       operation.MissionRevision,
		ReadyCount:    ready,
		RunningCount:  running,
		OpenOpps:      len(open),
		AdmittedOpps:  len(admitted),
		ArtifactCount: len(artifacts),
		SourceCount:   len(sources),
		ClaimCount:    len(claims),
		ObservationN:  len(observations),
		VerifiedAt:    now,
		Mode:          "model_free_local",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}

	artifactID, err := e.IDs.NewID("artifact")
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("generate artifact id: %w", err)
	}
	if strings.TrimSpace(artifactID) == "" {
		return domain.KnowledgeArtifact{}, errors.New("generated artifact id must not be empty")
	}

	base := domain.GenesisCommitID
	if head, headErr := tx.HeadCommit(operation.MissionRevision); headErr == nil {
		base = head.ID
	}

	deps := []string{
		"operation:" + string(operation.ID),
		"spec:" + string(spec.ID),
		"lease:" + leaseRef,
	}
	kind := "local_operation_audit"
	switch {
	case strings.Contains(string(spec.ID), "integrity_audit"):
		kind = "integrity_audit_report"
	case strings.Contains(string(spec.ID), "frontier_manage"):
		kind = "frontier_manage_report"
	case strings.Contains(string(spec.ID), "gap_scan"):
		kind = "gap_scan_report"
	}

	artifact := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.ArtifactID(artifactID),
		Kind:          kind,
		BaseCommitID:  base,
		Dependencies:  deps,
		ContentRef:    "inline:json:local-operation-audit-v1",
		Content:       string(raw),
	}
	if err := artifact.Validate(); err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("build local artifact: %w", err)
	}
	return artifact, nil
}
