package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/evaluation"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/view"
)

// Event kinds emitted by the model-free local executor.
const (
	EventOperationDispatched    = "operation.dispatched"
	EventOperationLocalVerified = "operation.local_verified"
	EventOperationSucceeded     = "operation.succeeded"
)

// defaultSourceFreshnessMaxAge is the model-free aging window for source_freshness.
// Sources whose newest SourceVersion.ObservedAt is older than now-window are
// reported as aging candidates (no automatic reacquisition in the local path).
const defaultSourceFreshnessMaxAge = 7 * 24 * time.Hour

// LocalExecutor runs continuity/local operations without a model provider.
// It is the first vertical of the architecture Executor: pure transitions under
// a lease reference, optional read-only audit artifact, and append-only events.
// Model-backed PROPOSE_ONLY paths remain out of scope until a provider is wired.
//
// Family-specific local effects (still model-free):
//   - artifact_refresh: mark non-audit KnowledgeArtifacts stale when BaseCommitID != head,
//     then regenerate a bounded batch of stale cited_claim_view successors (FR-KNOW-005)
//   - source_freshness: report sources whose newest version is outside the aging window
//   - integrity_audit: structural orphan / contradiction inventory (no auto-repair)
//   - conflict_evidence_review: unopposed and opposed claim inventory
//   - gap_scan / mission_coverage_scan: formal fragment→version→source coverage joins
//   - harness_evaluation: offline compile of cognitive-v1 matrix (no provider)
//   - frontier_management: signature/depth/family hygiene inventory
type LocalExecutor struct {
	Store    port.Store
	Clock    source.Clock
	IDs      source.IDGenerator
	MemoryStore port.MemoryReader
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
	// Web/file acquisition runs on dedicated executors (FR-RES-001/002), never the local path.
	if webCapabilityFromSpec(spec) != "" || fileCapabilityFromSpec(spec) != "" {
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

		leaseID, err := e.IDs.NewID("lease")
		if err != nil {
			return fmt.Errorf("generate lease id: %w", err)
		}
		if strings.TrimSpace(leaseID) == "" {
			return errors.New("generated lease id must not be empty")
		}
		now := e.Clock.Now().UTC()
		until := now.Add(e.leaseTTL())
		// Bind lease identity, attempt, and absolute deadline (FR-DUR-003).
		leaseRef := FormatLeaseRef(leaseID, operation.ID, operation.Attempt+1, until)
		result.LeaseRef = leaseRef

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
	Schema              string                   `json:"schema"`
	OperationID         domain.OperationID       `json:"operation_id"`
	SpecID              domain.OperationSpecID   `json:"spec_id"`
	Authority           domain.Authority         `json:"authority"`
	LeaseRef            string                   `json:"lease_ref"`
	Mission             domain.MissionRevisionID `json:"mission_revision_id"`
	ReadyCount          int                      `json:"ready_count"`
	RunningCount        int                      `json:"running_count"`
	OpenOpps            int                      `json:"open_opportunities"`
	AdmittedOpps        int                      `json:"admitted_opportunities"`
	ArtifactCount       int                      `json:"knowledge_artifact_count"`
	SourceCount         int                      `json:"source_count"`
	ClaimCount          int                      `json:"claim_count"`
	ObservationN        int                      `json:"observation_count"`
	VerifiedAt          time.Time                `json:"verified_at"`
	Mode                string                   `json:"mode"`
	Family              string                   `json:"family,omitempty"`
	DepthMax            int                      `json:"depth_max"`
	DepthHistogram      map[string]int           `json:"depth_histogram,omitempty"`
	OpenByFamily        map[string]int           `json:"open_by_family,omitempty"`
	AdmittedByFamily    map[string]int           `json:"admitted_by_family,omitempty"`
	SourcesWithoutObs   int                      `json:"sources_without_observation_count"`
	ClaimsWithoutEv     int                      `json:"claims_without_evidence_count"`
	FrontierDupes       int                      `json:"frontier_duplicate_signature_count"`
	HeadCommitID        domain.CommitID          `json:"head_commit_id,omitempty"`
	StaleBefore         int                      `json:"stale_artifacts_before,omitempty"`
	StaleMarked         int                      `json:"stale_artifacts_marked,omitempty"`
	RefreshRegenerated  int                      `json:"refresh_regenerated_count,omitempty"`
	OrphanEvidence      int                      `json:"orphan_evidence_links,omitempty"`
	OrphanObsAnchors    int                      `json:"orphan_observation_anchors,omitempty"`
	AgingSourceCount    int                      `json:"aging_source_count,omitempty"`
	FreshnessMaxAgeH    int                      `json:"freshness_max_age_hours,omitempty"`
	SourcesWithoutFrag  int                      `json:"sources_without_fragment_count,omitempty"`
	FragmentsWithoutObs int                      `json:"fragments_without_observation_count,omitempty"`
	HygieneDeferred     int                      `json:"hygiene_deferred_count,omitempty"`
	HygieneAbandoned    int                      `json:"hygiene_abandoned_count,omitempty"`
	HygieneSuperseded   int                      `json:"hygiene_superseded_count,omitempty"`
	HygieneReopened     int                      `json:"hygiene_reopened_count,omitempty"`
	HygieneActions      int                      `json:"hygiene_action_count,omitempty"`
	Findings            []string                 `json:"findings,omitempty"`
}

type localFamilyEffects struct {
	Findings            []string
	StaleBefore         int
	StaleMarked         int
	RefreshRegenerated  int
	OrphanEvidence      int
	OrphanObsAnchors    int
	AgingSourceCount    int
	FreshnessMaxAgeH    int
	SourcesWithoutFrag  int
	FragmentsWithoutObs int
	HygieneDeferred     int
	HygieneAbandoned    int
	HygieneSuperseded   int
	HygieneReopened     int
	HygieneActions      int
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
	// frontier_management: apply pure reservoir hygiene (supersede/defer/abandon/reopen)
	// under HorizonPolicy before inventory metrics are captured for the audit artifact.
	familyEarly := familyFromSpec(spec.ID)
	var hygieneEffects localFamilyEffects
	if familyEarly == string(domain.FamilyFrontierManage) {
		deferred, derr := tx.WorkOpportunities(operation.MissionRevision, domain.OpportunityDeferred)
		if derr != nil {
			return domain.KnowledgeArtifact{}, derr
		}
		hygieneEffects, err = applyFrontierHygieneEffects(tx, operation, open, deferred, now)
		if err != nil {
			return domain.KnowledgeArtifact{}, err
		}
		open, err = tx.WorkOpportunities(operation.MissionRevision, domain.OpportunityOpen)
		if err != nil {
			return domain.KnowledgeArtifact{}, err
		}
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
	evidence, err := tx.EvidenceLinks()
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}

	// Residual family depth: frontier metrics + knowledge coverage hints.
	depthHist := map[string]int{}
	openByFamily := map[string]int{}
	admittedByFamily := map[string]int{}
	sigCount := map[string]int{}
	depthMax := 0
	for _, opp := range open {
		openByFamily[string(opp.Family)]++
		depthHist[fmt.Sprintf("%d", opp.Depth)]++
		if opp.Depth > depthMax {
			depthMax = opp.Depth
		}
		sigCount[opp.DedupSignature]++
	}
	for _, opp := range admitted {
		admittedByFamily[string(opp.Family)]++
		depthHist[fmt.Sprintf("%d", opp.Depth)]++
		if opp.Depth > depthMax {
			depthMax = opp.Depth
		}
		sigCount[opp.DedupSignature]++
	}
	dupes := 0
	for _, n := range sigCount {
		if n > 1 {
			dupes += n - 1
		}
	}
	claimHasEvidence := map[string]struct{}{}
	for _, link := range evidence {
		if link.ClaimID != "" {
			claimHasEvidence[string(link.ClaimID)] = struct{}{}
		}
	}
	claimsWithoutEv := 0
	for _, claim := range claims {
		if _, ok := claimHasEvidence[string(claim.ID)]; !ok {
			claimsWithoutEv++
		}
	}

	// Join observation anchors through fragment → version → source for coverage/gap.
	fragmentByID := map[domain.SourceFragmentID]domain.SourceFragment{}
	versionByID := map[domain.SourceVersionID]domain.SourceVersion{}
	versions, err := tx.SourceVersions("")
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	for _, ver := range versions {
		versionByID[ver.ID] = ver
		frags, fragErr := tx.SourceFragments(ver.ID)
		if fragErr != nil {
			return domain.KnowledgeArtifact{}, fragErr
		}
		for _, frag := range frags {
			fragmentByID[frag.ID] = frag
		}
	}
	observedSourceIDs, sourcesWithoutObs, sourcesWithoutFrag, fragsWithoutObs :=
		coverageJoin(sources, versionByID, fragmentByID, observations)
	_ = observedSourceIDs

	obsByID := map[domain.ObservationID]domain.Observation{}
	for _, obs := range observations {
		obsByID[obs.ID] = obs
	}
	claimByID := map[domain.ClaimID]domain.Claim{}
	for _, claim := range claims {
		claimByID[claim.ID] = claim
	}

	family := familyEarly
	if family == "" {
		family = familyFromSpec(spec.ID)
	}

	headID := domain.GenesisCommitID
	if head, headErr := tx.HeadCommit(operation.MissionRevision); headErr == nil {
		headID = head.ID
	}

	var missionDomains []string
	if rev, revErr := tx.MissionRevision(operation.MissionRevision); revErr == nil {
		missionDomains = append([]string(nil), rev.Domains...)
	}

	// Family-specific local effects (read-mostly; artifact_refresh may mark stale).
	effects, err := applyLocalFamilyEffects(tx, family, now, headID, artifacts, sources, versions, versionByID, fragmentByID, observations, obsByID, claims, claimByID, evidence, sourcesWithoutObs, sourcesWithoutFrag, fragsWithoutObs, claimsWithoutEv, dupes, depthMax, openByFamily, missionDomains)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	// Authorized regeneration after stale-mark (store-retention.v1 / FR-KNOW-005).
	if family == string(domain.FamilyArtifactRefresh) {
		created, regenErr := view.RefreshCitedBatchInTx(tx, e.IDs, headID, view.DefaultRefreshBatch)
		if regenErr != nil {
			return domain.KnowledgeArtifact{}, fmt.Errorf("artifact_refresh regenerate cited views: %w", regenErr)
		}
		effects.RefreshRegenerated = len(created)
		if len(created) == 0 {
			effects.Findings = append(effects.Findings, "refresh:no_cited_view_regenerated")
		} else {
			for _, art := range created {
				effects.Findings = append(effects.Findings, fmt.Sprintf("refresh:regenerated=%s base=%s", art.ID, art.BaseCommitID))
			}
		}
		effects.Findings = append(effects.Findings, fmt.Sprintf("refresh:regenerated_count=%d", len(created)))
	}
	// Merge hygiene write-path counters/findings produced before inventory.
	effects.HygieneDeferred = hygieneEffects.HygieneDeferred
	effects.HygieneAbandoned = hygieneEffects.HygieneAbandoned
	effects.HygieneSuperseded = hygieneEffects.HygieneSuperseded
	effects.HygieneReopened = hygieneEffects.HygieneReopened
	effects.HygieneActions = hygieneEffects.HygieneActions
	if len(hygieneEffects.Findings) > 0 {
		effects.Findings = append(append([]string(nil), hygieneEffects.Findings...), effects.Findings...)
	}

	body := localAuditBody{
		Schema:              "local-operation-audit-v1",
		OperationID:         operation.ID,
		SpecID:              spec.ID,
		Authority:           spec.MaximumAuthority,
		LeaseRef:            leaseRef,
		Mission:             operation.MissionRevision,
		ReadyCount:          ready,
		RunningCount:        running,
		OpenOpps:            len(open),
		AdmittedOpps:        len(admitted),
		ArtifactCount:       len(artifacts),
		SourceCount:         len(sources),
		ClaimCount:          len(claims),
		ObservationN:        len(observations),
		VerifiedAt:          now,
		Mode:                "model_free_local",
		Family:              family,
		DepthMax:            depthMax,
		DepthHistogram:      depthHist,
		OpenByFamily:        openByFamily,
		AdmittedByFamily:    admittedByFamily,
		SourcesWithoutObs:   sourcesWithoutObs,
		ClaimsWithoutEv:     claimsWithoutEv,
		FrontierDupes:       dupes,
		HeadCommitID:        headID,
		StaleBefore:         effects.StaleBefore,
		StaleMarked:         effects.StaleMarked,
		RefreshRegenerated:  effects.RefreshRegenerated,
		OrphanEvidence:      effects.OrphanEvidence,
		OrphanObsAnchors:    effects.OrphanObsAnchors,
		AgingSourceCount:    effects.AgingSourceCount,
		FreshnessMaxAgeH:    effects.FreshnessMaxAgeH,
		SourcesWithoutFrag:  effects.SourcesWithoutFrag,
		FragmentsWithoutObs: effects.FragmentsWithoutObs,
		HygieneDeferred:     effects.HygieneDeferred,
		HygieneAbandoned:    effects.HygieneAbandoned,
		HygieneSuperseded:   effects.HygieneSuperseded,
		HygieneReopened:     effects.HygieneReopened,
		HygieneActions:      effects.HygieneActions,
		Findings:            effects.Findings,
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
	case strings.Contains(string(spec.ID), "coverage"):
		kind = "coverage_scan_report"
	case strings.Contains(string(spec.ID), "artifact_refresh"):
		kind = "artifact_refresh_report"
	case strings.Contains(string(spec.ID), "source_freshness"):
		kind = "source_freshness_report"
	case strings.Contains(string(spec.ID), "conflict"):
		kind = "conflict_review_report"
	case strings.Contains(string(spec.ID), "harness"):
		kind = "harness_evaluation_report"
	}

	artifact := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.ArtifactID(artifactID),
		Kind:          kind,
		BaseCommitID:  headID,
		Dependencies:  deps,
		ContentRef:    "inline:json:local-operation-audit-v1",
		Content:       string(raw),
	}
	if err := artifact.Validate(); err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("build local artifact: %w", err)
	}
	return artifact, nil
}

func familyFromSpec(id domain.OperationSpecID) string {
	s := string(id)
	switch {
	case strings.Contains(s, "gap_scan"):
		return string(domain.FamilyGapScan)
	case strings.Contains(s, "conflict"):
		return string(domain.FamilyConflictReview)
	case strings.Contains(s, "artifact_refresh"):
		return string(domain.FamilyArtifactRefresh)
	case strings.Contains(s, "integrity_audit"):
		return string(domain.FamilyIntegrityAudit)
	case strings.Contains(s, "harness"):
		return string(domain.FamilyHarnessEvaluation)
	case strings.Contains(s, "frontier"):
		return string(domain.FamilyFrontierManage)
	case strings.Contains(s, "coverage"):
		return string(domain.FamilyCoverageScan)
	case strings.Contains(s, "source_freshness"):
		return string(domain.FamilySourceFreshness)
	default:
		return ""
	}
}

// coverageJoin resolves observation anchors through fragment → version → source.
// It returns the set of sources that have at least one observation, sources with
// no observation, sources with no fragments at all, and fragments with no observation.
func coverageJoin(
	sources []domain.Source,
	versionByID map[domain.SourceVersionID]domain.SourceVersion,
	fragmentByID map[domain.SourceFragmentID]domain.SourceFragment,
	observations []domain.Observation,
) (observed map[domain.SourceID]struct{}, withoutObs, withoutFrag, fragsWithoutObs int) {
	observed = map[domain.SourceID]struct{}{}
	observedFrags := map[domain.SourceFragmentID]struct{}{}
	for _, obs := range observations {
		fid := obs.Anchor.SourceFragmentID
		if fid == "" {
			continue
		}
		observedFrags[fid] = struct{}{}
		frag, ok := fragmentByID[fid]
		if !ok {
			continue
		}
		ver, ok := versionByID[frag.SourceVersionID]
		if !ok {
			continue
		}
		observed[ver.SourceID] = struct{}{}
	}
	sourceHasFrag := map[domain.SourceID]bool{}
	for _, frag := range fragmentByID {
		ver, ok := versionByID[frag.SourceVersionID]
		if !ok {
			continue
		}
		sourceHasFrag[ver.SourceID] = true
		if _, hasObs := observedFrags[frag.ID]; !hasObs {
			fragsWithoutObs++
		}
	}
	for _, src := range sources {
		if !sourceHasFrag[src.ID] {
			withoutFrag++
		}
		if _, ok := observed[src.ID]; !ok {
			withoutObs++
		}
	}
	return observed, withoutObs, withoutFrag, fragsWithoutObs
}

func applyLocalFamilyEffects(
	tx port.Transaction,
	family string,
	now time.Time,
	headID domain.CommitID,
	artifacts []domain.KnowledgeArtifact,
	sources []domain.Source,
	versions []domain.SourceVersion,
	versionByID map[domain.SourceVersionID]domain.SourceVersion,
	fragmentByID map[domain.SourceFragmentID]domain.SourceFragment,
	observations []domain.Observation,
	obsByID map[domain.ObservationID]domain.Observation,
	claims []domain.Claim,
	claimByID map[domain.ClaimID]domain.Claim,
	evidence []domain.EvidenceLink,
	sourcesWithoutObs, sourcesWithoutFrag, fragsWithoutObs, claimsWithoutEv, dupes, depthMax int,
	openByFamily map[string]int,
	missionDomains []string,
) (localFamilyEffects, error) {
	var out localFamilyEffects
	out.SourcesWithoutFrag = sourcesWithoutFrag
	out.FragmentsWithoutObs = fragsWithoutObs

	// Shared structural integrity counters.
	orphanEvidence := 0
	for _, link := range evidence {
		if _, ok := obsByID[link.ObservationID]; !ok {
			orphanEvidence++
			continue
		}
		if _, ok := claimByID[link.ClaimID]; !ok {
			orphanEvidence++
		}
	}
	orphanObs := 0
	for _, obs := range observations {
		if obs.Anchor.SourceFragmentID != "" {
			if _, ok := fragmentByID[obs.Anchor.SourceFragmentID]; !ok {
				orphanObs++
			}
		}
	}
	out.OrphanEvidence = orphanEvidence
	out.OrphanObsAnchors = orphanObs

	// Newest version per source for freshness.
	newestBySource := map[domain.SourceID]domain.SourceVersion{}
	for _, ver := range versions {
		prev, ok := newestBySource[ver.SourceID]
		if !ok || ver.ObservedAt.After(prev.ObservedAt) || (ver.ObservedAt.Equal(prev.ObservedAt) && string(ver.ID) > string(prev.ID)) {
			newestBySource[ver.SourceID] = ver
		}
	}

	switch family {
	case string(domain.FamilyArtifactRefresh):
		staleBefore := 0
		for _, a := range artifacts {
			if a.Stale {
				staleBefore++
			}
		}
		out.StaleBefore = staleBefore
		// Mark non-stale knowledge artifacts whose BaseCommitID is not the
		// current mission head. Audit/report artifacts stay fresh so local
		// scans do not invalidate their own trail.
		marked := 0
		for _, a := range artifacts {
			if a.Stale {
				continue
			}
			if isLocalAuditKind(a.Kind) {
				continue
			}
			if a.BaseCommitID == headID {
				continue
			}
			updated := a
			updated.Stale = true
			if err := tx.SaveKnowledgeArtifact(updated); err != nil {
				return out, fmt.Errorf("mark artifact %s stale: %w", a.ID, err)
			}
			marked++
			out.Findings = append(out.Findings, fmt.Sprintf("refresh:marked_stale=%s base=%s head=%s", a.ID, a.BaseCommitID, headID))
		}
		out.StaleMarked = marked
		if marked == 0 {
			out.Findings = append(out.Findings, "refresh:no_artifact_required_stale_transition")
		}
		out.Findings = append(out.Findings, fmt.Sprintf("refresh:stale_before=%d", staleBefore))
		out.Findings = append(out.Findings, fmt.Sprintf("refresh:stale_marked=%d", marked))
		out.Findings = append(out.Findings, fmt.Sprintf("refresh:head=%s", headID))

	case string(domain.FamilySourceFreshness):
		window := defaultSourceFreshnessMaxAge
		out.FreshnessMaxAgeH = int(window / time.Hour)
		cutoff := now.Add(-window)
		aging := 0
		for _, src := range sources {
			ver, ok := newestBySource[src.ID]
			observed := src.ObservedAt
			if ok {
				observed = ver.ObservedAt
			}
			if observed.Before(cutoff) {
				aging++
				out.Findings = append(out.Findings, fmt.Sprintf("freshness:aging_source=%s observed_at=%s", src.ID, observed.UTC().Format(time.RFC3339)))
			}
		}
		out.AgingSourceCount = aging
		if aging == 0 {
			out.Findings = append(out.Findings, "freshness:no_aging_sources_in_window")
		}
		out.Findings = append(out.Findings, fmt.Sprintf("freshness:window_hours=%d", out.FreshnessMaxAgeH))
		out.Findings = append(out.Findings, fmt.Sprintf("freshness:aging_count=%d", aging))
		if sourcesWithoutObs > 0 {
			out.Findings = append(out.Findings, fmt.Sprintf("freshness:sources_lacking_observation_anchor=%d", sourcesWithoutObs))
		}

	case string(domain.FamilyIntegrityAudit):
		// Structural referential checks only (no model, no auto-repair).
		if orphanEvidence > 0 {
			out.Findings = append(out.Findings, fmt.Sprintf("integrity:orphan_evidence_links=%d", orphanEvidence))
		}
		if orphanObs > 0 {
			out.Findings = append(out.Findings, fmt.Sprintf("integrity:orphan_observation_fragment_anchors=%d", orphanObs))
		}
		// Contradiction pairs: claims with both SUPPORTS and CONTRADICTS evidence.
		support := map[domain.ClaimID]int{}
		contradict := map[domain.ClaimID]int{}
		for _, link := range evidence {
			switch link.Relation {
			case domain.EvidenceSupports:
				support[link.ClaimID]++
			case domain.EvidenceContradicts:
				contradict[link.ClaimID]++
			}
		}
		conflicted := 0
		for id, n := range support {
			if contradict[id] > 0 {
				conflicted++
				out.Findings = append(out.Findings, fmt.Sprintf("integrity:claim_with_support_and_contradict=%s support=%d contradict=%d", id, n, contradict[id]))
			}
		}
		if claimsWithoutEv > 0 {
			out.Findings = append(out.Findings, fmt.Sprintf("integrity:claims_without_evidence=%d", claimsWithoutEv))
		}
		if orphanEvidence == 0 && orphanObs == 0 && conflicted == 0 && claimsWithoutEv == 0 {
			out.Findings = append(out.Findings, "integrity:no_structural_issues")
		}
		out.Findings = append(out.Findings, fmt.Sprintf("integrity:conflicted_claims=%d", conflicted))

	case string(domain.FamilyConflictReview):
		// Inventory unopposed claims and explicit contradiction pairs (read-only).
		hasSupport := map[domain.ClaimID]bool{}
		hasContradict := map[domain.ClaimID]bool{}
		for _, link := range evidence {
			switch link.Relation {
			case domain.EvidenceSupports, domain.EvidenceReplicates:
				hasSupport[link.ClaimID] = true
			case domain.EvidenceContradicts, domain.EvidenceFailsToReplicate:
				hasContradict[link.ClaimID] = true
			}
		}
		unopposed := 0
		for _, claim := range claims {
			if hasSupport[claim.ID] && !hasContradict[claim.ID] {
				unopposed++
			}
		}
		conflicted := 0
		for id := range hasSupport {
			if hasContradict[id] {
				conflicted++
			}
		}
		out.Findings = append(out.Findings, fmt.Sprintf("conflict:unopposed_supported_claims=%d", unopposed))
		out.Findings = append(out.Findings, fmt.Sprintf("conflict:claims_with_support_and_opposition=%d", conflicted))
		if claimsWithoutEv > 0 {
			out.Findings = append(out.Findings, fmt.Sprintf("conflict:claims_without_evidence=%d", claimsWithoutEv))
		}
		if unopposed == 0 && conflicted == 0 && claimsWithoutEv == 0 {
			out.Findings = append(out.Findings, "conflict:no_review_candidates")
		}

	case string(domain.FamilyGapScan):
		// Formal join: sources without observation, sources without fragments,
		// and fragments without observation. Enumerate up to a small cap so the
		// audit is actionable without dumping unbounded inventories.
		const capIDs = 8
		observed, _, _, _ := coverageJoin(sources, versionByID, fragmentByID, observations)
		listed := 0
		for _, src := range sources {
			if _, ok := observed[src.ID]; ok {
				continue
			}
			if listed < capIDs {
				out.Findings = append(out.Findings, fmt.Sprintf("gap:source_without_observation=%s", src.ID))
				listed++
			}
		}
		out.Findings = append(out.Findings, fmt.Sprintf("gap:sources_without_observation=%d", sourcesWithoutObs))
		out.Findings = append(out.Findings, fmt.Sprintf("gap:sources_without_fragment=%d", sourcesWithoutFrag))
		out.Findings = append(out.Findings, fmt.Sprintf("gap:fragments_without_observation=%d", fragsWithoutObs))
		if claimsWithoutEv > 0 {
			out.Findings = append(out.Findings, fmt.Sprintf("gap:claims_without_evidence=%d", claimsWithoutEv))
		}
		if sourcesWithoutObs == 0 && sourcesWithoutFrag == 0 && fragsWithoutObs == 0 && claimsWithoutEv == 0 {
			out.Findings = append(out.Findings, "gap:no_structural_gaps")
		}

	case string(domain.FamilyCoverageScan):
		// Mission-domain tags plus structural coverage via the same join.
		out.Findings = append(out.Findings, fmt.Sprintf("coverage:mission_domains=%d", len(missionDomains)))
		for i, d := range missionDomains {
			if i >= 8 {
				out.Findings = append(out.Findings, fmt.Sprintf("coverage:mission_domains_truncated=%d", len(missionDomains)-8))
				break
			}
			out.Findings = append(out.Findings, fmt.Sprintf("coverage:mission_domain=%s", d))
		}
		out.Findings = append(out.Findings, fmt.Sprintf("coverage:sources_without_observation=%d", sourcesWithoutObs))
		out.Findings = append(out.Findings, fmt.Sprintf("coverage:sources_without_fragment=%d", sourcesWithoutFrag))
		out.Findings = append(out.Findings, fmt.Sprintf("coverage:fragments_without_observation=%d", fragsWithoutObs))
		if claimsWithoutEv > 0 {
			out.Findings = append(out.Findings, fmt.Sprintf("coverage:claims_without_evidence=%d", claimsWithoutEv))
		}
		if sourcesWithoutObs == 0 && sourcesWithoutFrag == 0 && fragsWithoutObs == 0 && claimsWithoutEv == 0 {
			out.Findings = append(out.Findings, "coverage:no_structural_gap")
		}

	case string(domain.FamilyHarnessEvaluation):
		// Provider-free compile of the embedded cognitive fixture matrix.
		fixtures, err := evaluation.LoadEmbeddedCognitiveV1()
		if err != nil {
			return out, fmt.Errorf("load embedded cognitive fixtures: %w", err)
		}
		report, err := evaluation.CompileMatrix(fixtures, evaluation.DefaultCognitiveMatrix(), prompt.ConservativeEstimator{}, evaluation.DefaultOperationSpec())
		if err != nil {
			return out, fmt.Errorf("offline harness compile: %w", err)
		}
		out.Findings = append(out.Findings, evaluation.OfflineFindings(report)...)

	case string(domain.FamilyFrontierManage):
		out.Findings = append(out.Findings, frontierHygieneFindings(dupes, depthMax, openByFamily)...)

	default:
		out.Findings = residualFindings(family, sourcesWithoutObs, claimsWithoutEv, dupes, depthMax, openByFamily)
	}
	return out, nil
}

// applyFrontierHygieneEffects plans and persists pure WorkOpportunity lifecycle
// transitions for reservoir hygiene (signature supersede, depth abandon, defer,
// reopen). ADMITTED stays under Admitter authority. Policy comes from the active
// HORIZON revision when present, else DefaultHorizonPolicy. Returns counters/
// findings; does not invent work or grant model admission power.
func applyFrontierHygieneEffects(
	tx port.Transaction,
	operation domain.Operation,
	open []domain.WorkOpportunity,
	deferred []domain.WorkOpportunity,
	now time.Time,
) (localFamilyEffects, error) {
	var out localFamilyEffects
	policy, err := horizonPolicyFromTx(tx)
	if err != nil {
		return out, err
	}
	actions, err := domain.PlanFrontierReservoirHygiene(open, deferred, policy, now)
	if err != nil {
		return out, fmt.Errorf("plan frontier hygiene: %w", err)
	}
	if len(actions) == 0 {
		out.Findings = append(out.Findings, "frontier:hygiene_noop")
		return out, nil
	}

	byID := make(map[domain.WorkOpportunityID]domain.WorkOpportunity, len(open)+len(deferred))
	for _, opp := range open {
		byID[opp.ID] = opp
	}
	for _, opp := range deferred {
		byID[opp.ID] = opp
	}

	for i, action := range actions {
		current, ok := byID[action.OpportunityID]
		if !ok {
			// Reload if not in the pre-plan snapshot (should not happen for pure plan).
			loaded, loadErr := tx.WorkOpportunity(action.OpportunityID)
			if loadErr != nil {
				return out, fmt.Errorf("load opportunity %s for hygiene: %w", action.OpportunityID, loadErr)
			}
			current = loaded
		}
		next, trErr := domain.TransitionWorkOpportunity(current, action.Transition)
		if trErr != nil {
			return out, fmt.Errorf("transition work opportunity %s: %w", action.OpportunityID, trErr)
		}
		if err := tx.SaveWorkOpportunity(next); err != nil {
			return out, fmt.Errorf("save hygiene transition %s: %w", next.ID, err)
		}
		byID[next.ID] = next

		kind, kindErr := domain.EventKindForOpportunityTransition(action.Transition.Event)
		if kindErr != nil {
			return out, kindErr
		}
		eventID := domain.EventID(fmt.Sprintf("%s:hygiene:%s:%d", operation.ID, next.ID, i))
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              eventID,
			Kind:            kind,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      string(next.ID) + ";" + string(action.Transition.Event),
		}); err != nil {
			return out, fmt.Errorf("append hygiene event: %w", err)
		}

		out.HygieneActions++
		switch action.Transition.Event {
		case domain.OppEventDefer:
			out.HygieneDeferred++
			out.Findings = append(out.Findings, fmt.Sprintf("frontier:deferred=%s", next.ID))
		case domain.OppEventAbandon:
			out.HygieneAbandoned++
			out.Findings = append(out.Findings, fmt.Sprintf("frontier:abandoned=%s", next.ID))
		case domain.OppEventSupersede:
			out.HygieneSuperseded++
			out.Findings = append(out.Findings, fmt.Sprintf("frontier:superseded=%s", next.ID))
		case domain.OppEventReopen:
			out.HygieneReopened++
			out.Findings = append(out.Findings, fmt.Sprintf("frontier:reopened=%s", next.ID))
		}
	}

	// Compact summary event for inspect/dashboards (one per hygiene apply).
	if _, err := tx.AppendEvent(domain.Event{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              domain.EventID(fmt.Sprintf("%s:frontier_compacted:%d", operation.ID, operation.Attempt+1)),
		Kind:            domain.EventContinuityFrontierCompacted,
		OccurredAt:      now,
		MissionRevision: operation.MissionRevision,
		InquiryID:       operation.InquiryID,
		OperationID:     operation.ID,
		PayloadRef: fmt.Sprintf(
			"actions=%d deferred=%d abandoned=%d superseded=%d reopened=%d max_candidates=%d max_depth=%d",
			out.HygieneActions, out.HygieneDeferred, out.HygieneAbandoned, out.HygieneSuperseded, out.HygieneReopened,
			policy.MaxCandidates, policy.MaxDepth,
		),
	}); err != nil {
		return out, fmt.Errorf("append frontier compact event: %w", err)
	}
	out.Findings = append(out.Findings,
		fmt.Sprintf("frontier:hygiene_actions=%d", out.HygieneActions),
		fmt.Sprintf("frontier:hygiene_deferred=%d", out.HygieneDeferred),
		fmt.Sprintf("frontier:hygiene_abandoned=%d", out.HygieneAbandoned),
		fmt.Sprintf("frontier:hygiene_superseded=%d", out.HygieneSuperseded),
		fmt.Sprintf("frontier:hygiene_reopened=%d", out.HygieneReopened),
	)
	return out, nil
}

func horizonPolicyFromTx(tx port.Transaction) (domain.HorizonPolicy, error) {
	revision, err := tx.ActiveConfigRevision(domain.ConfigScopeHorizon)
	if errors.Is(err, port.ErrNotFound) {
		return domain.DefaultHorizonPolicy(), nil
	}
	if err != nil {
		return domain.HorizonPolicy{}, err
	}
	if revision.Horizon == nil {
		return domain.HorizonPolicy{}, fmt.Errorf("active horizon revision %s has no payload", revision.ID)
	}
	policy := *revision.Horizon
	if err := policy.Validate(); err != nil {
		return domain.HorizonPolicy{}, err
	}
	return policy, nil
}

func isLocalAuditKind(kind string) bool {
	return domain.IsLocalAuditArtifactKind(kind)
}

func frontierHygieneFindings(dupes, depthMax int, openByFamily map[string]int) []string {
	out := []string{
		fmt.Sprintf("frontier:duplicate_signatures=%d", dupes),
		fmt.Sprintf("frontier:depth_max=%d", depthMax),
		fmt.Sprintf("frontier:open_family_count=%d", len(openByFamily)),
	}
	openTotal := 0
	families := make([]string, 0, len(openByFamily))
	for fam := range openByFamily {
		families = append(families, fam)
	}
	sort.Strings(families)
	for i, fam := range families {
		n := openByFamily[fam]
		openTotal += n
		if i < 12 {
			out = append(out, fmt.Sprintf("frontier:open_%s=%d", fam, n))
		}
	}
	if len(families) > 12 {
		out = append(out, fmt.Sprintf("frontier:open_families_truncated=%d", len(families)-12))
	}
	out = append(out, fmt.Sprintf("frontier:open_total=%d", openTotal))
	if openTotal == 0 {
		out = append(out, "frontier:no_open_opportunities")
	} else if dupes == 0 {
		out = append(out, "frontier:signatures_unique")
	} else {
		out = append(out, "frontier:signature_dedupe_candidates")
	}
	return out
}

func residualFindings(family string, sourcesWithoutObs, claimsWithoutEv, dupes, depthMax int, openByFamily map[string]int) []string {
	var out []string
	switch family {
	case string(domain.FamilyCoverageScan):
		// Legacy residual path; applyLocalFamilyEffects owns coverage joins now.
		if sourcesWithoutObs > 0 {
			out = append(out, fmt.Sprintf("coverage:sources_without_observation=%d", sourcesWithoutObs))
		}
		if claimsWithoutEv > 0 {
			out = append(out, fmt.Sprintf("coverage:claims_without_evidence=%d", claimsWithoutEv))
		}
		if sourcesWithoutObs == 0 && claimsWithoutEv == 0 {
			out = append(out, "coverage:no_structural_gap")
		}
	case string(domain.FamilySourceFreshness):
		// Legacy residual path; applyLocalFamilyEffects handles this family now.
		if sourcesWithoutObs > 0 {
			out = append(out, fmt.Sprintf("freshness:sources_lacking_observation_anchor=%d", sourcesWithoutObs))
		} else {
			out = append(out, "freshness:all_sources_have_observation_hint_or_none")
		}
	case string(domain.FamilyFrontierManage):
		// Legacy residual path; applyLocalFamilyEffects owns frontier hygiene now.
		out = append(out, frontierHygieneFindings(dupes, depthMax, openByFamily)...)
	default:
		if depthMax > 0 {
			out = append(out, fmt.Sprintf("depth_max=%d", depthMax))
		}
	}
	return out
}
