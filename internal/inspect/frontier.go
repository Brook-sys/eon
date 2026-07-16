package inspect

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// DefaultFrontierListLimit caps opportunity browse lists for the Control API.
const DefaultFrontierListLimit = 50

// MaxFrontierListLimit is the hard ceiling for frontier list endpoints.
const MaxFrontierListLimit = 200

// MaxFrontierHygieneActions caps dry-run action rows returned to operators.
const MaxFrontierHygieneActions = 64

// opportunityTextMax bounds free-text opportunity fields in presentation.
const opportunityTextMax = 512

// WorkOpportunitySummary is a compact, redacted frontier row.
type WorkOpportunitySummary struct {
	ID              domain.WorkOpportunityID     `json:"id"`
	MissionRevision domain.MissionRevisionID     `json:"mission_revision_id"`
	Family          domain.WorkFamily            `json:"family"`
	Status          domain.WorkOpportunityStatus `json:"status"`
	Title           string                       `json:"title"`
	Origin          string                       `json:"origin"`
	ExpectedGain    string                       `json:"expected_gain,omitempty"`
	Novelty         string                       `json:"novelty,omitempty"`
	StopCondition   string                       `json:"stop_condition,omitempty"`
	DedupSignature  string                       `json:"dedup_signature"`
	ParentID        domain.WorkOpportunityID     `json:"parent_id,omitempty"`
	Depth           int                          `json:"depth"`
	Priority        uint8                        `json:"priority"`
	Risk            domain.RiskLevel             `json:"risk"`
	CreatedAt       time.Time                    `json:"created_at"`
	UpdatedAt       time.Time                    `json:"updated_at"`
	AdmittedInquiry domain.InquiryID             `json:"admitted_inquiry_id,omitempty"`
	AbandonReason   string                       `json:"abandon_reason,omitempty"`
	// OverDepth is true when Depth exceeds the active HorizonPolicy.MaxDepth.
	OverDepth bool `json:"over_depth,omitempty"`
}

// FrontierPage is a paginated opportunity browse response.
type FrontierPage struct {
	SchemaVersion   int                          `json:"schema_version"`
	MissionID       domain.MissionID             `json:"mission_id"`
	MissionRevision domain.MissionRevisionID     `json:"mission_revision_id"`
	Total           int                          `json:"total"`
	Limit           int                          `json:"limit"`
	Offset          int                          `json:"offset"`
	StatusFilter    domain.WorkOpportunityStatus `json:"status_filter,omitempty"`
	FamilyFilter    domain.WorkFamily            `json:"family_filter,omitempty"`
	PolicyVersion   string                       `json:"policy_version"`
	MaxCandidates   int                          `json:"max_candidates"`
	MaxDepth        int                          `json:"max_depth"`
	Items           []WorkOpportunitySummary     `json:"items"`
}

// WorkOpportunityDetail is a single opportunity plus lineage/siblings counts.
type WorkOpportunityDetail struct {
	SchemaVersion  int                    `json:"schema_version"`
	Opportunity    WorkOpportunitySummary `json:"opportunity"`
	PolicyVersion  string                 `json:"policy_version"`
	MaxDepth       int                    `json:"max_depth"`
	MaxCandidates  int                    `json:"max_candidates"`
	ChildrenCount  int                    `json:"children_count"`
	SiblingCount   int                    `json:"sibling_count"`
	SignaturePeers int                    `json:"signature_peers"`
	CanSpawnChild  bool                   `json:"can_spawn_child"`
	SpawnBlocker   string                 `json:"spawn_blocker,omitempty"`
	Redaction      RedactionReport        `json:"redaction"`
}

// FrontierHygieneActionSummary is a presentation row for a pure dry-run action.
type FrontierHygieneActionSummary struct {
	OpportunityID  domain.WorkOpportunityID              `json:"opportunity_id"`
	Event          domain.WorkOpportunityTransitionEvent `json:"event"`
	Reason         string                                `json:"reason,omitempty"`
	SupersededBy   domain.WorkOpportunityID              `json:"superseded_by,omitempty"`
	Family         domain.WorkFamily                     `json:"family,omitempty"`
	Priority       uint8                                 `json:"priority,omitempty"`
	Depth          int                                   `json:"depth,omitempty"`
	StatusBefore   domain.WorkOpportunityStatus          `json:"status_before,omitempty"`
	DedupSignature string                                `json:"dedup_signature,omitempty"`
}

// FrontierHygieneProjection is a read-only dry-run of PlanFrontierReservoirHygiene.
// It never mutates the store; operators use it to anticipate compact effects.
type FrontierHygieneProjection struct {
	SchemaVersion    int                            `json:"schema_version"`
	MissionID        domain.MissionID               `json:"mission_id"`
	MissionRevision  domain.MissionRevisionID       `json:"mission_revision_id"`
	ObservedAt       time.Time                      `json:"observed_at"`
	PolicyVersion    string                         `json:"policy_version"`
	MaxCandidates    int                            `json:"max_candidates"`
	MaxDepth         int                            `json:"max_depth"`
	OpenBefore       int                            `json:"open_before"`
	DeferredBefore   int                            `json:"deferred_before"`
	UniqueSignatures int                            `json:"unique_signatures"`
	DuplicateGroups  int                            `json:"duplicate_signature_groups"`
	OverDepthOpen    int                            `json:"over_depth_open"`
	NeedsCompact     bool                           `json:"needs_compact"`
	ActionCount      int                            `json:"action_count"`
	ActionsTruncated int                            `json:"actions_truncated,omitempty"`
	DeferredCount    int                            `json:"hygiene_deferred_count"`
	AbandonedCount   int                            `json:"hygiene_abandoned_count"`
	SupersededCount  int                            `json:"hygiene_superseded_count"`
	ReopenedCount    int                            `json:"hygiene_reopened_count"`
	Actions          []FrontierHygieneActionSummary `json:"actions"`
	Findings         []string                       `json:"findings,omitempty"`
}

// FrontierListFilter constrains opportunity browse.
type FrontierListFilter struct {
	Status domain.WorkOpportunityStatus
	Family domain.WorkFamily
	Limit  int
	Offset int
}

// ListFrontier returns a stable, filtered opportunity page for a mission's active revision.
func (p *Projector) ListFrontier(ctx context.Context, missionID domain.MissionID, filter FrontierListFilter) (FrontierPage, error) {
	if missionID == "" {
		return FrontierPage{}, errors.New("mission ID is required")
	}
	limit, offset, err := normalizeFrontierPage(filter.Limit, filter.Offset)
	if err != nil {
		return FrontierPage{}, err
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return FrontierPage{}, fmt.Errorf("unknown work opportunity status filter %q", filter.Status)
	}
	if filter.Family != "" && !filter.Family.Valid() {
		return FrontierPage{}, fmt.Errorf("unknown work family filter %q", filter.Family)
	}

	var page FrontierPage
	err = p.Store.View(ctx, func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(missionID)
		if err != nil {
			return err
		}
		policy, err := resolveHorizonPolicy(r)
		if err != nil {
			return err
		}
		opps, err := r.WorkOpportunities(active.ID, filter.Status)
		if err != nil {
			return err
		}
		items := make([]WorkOpportunitySummary, 0, len(opps))
		for _, opp := range opps {
			if filter.Family != "" && opp.Family != filter.Family {
				continue
			}
			items = append(items, summarizeOpportunity(opp, policy.MaxDepth))
		}
		// Stable order: priority desc, updated_at desc, id asc (store already
		// sorts by priority/created; re-sort for inspect presentation).
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Priority != items[j].Priority {
				return items[i].Priority > items[j].Priority
			}
			if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
				return items[i].UpdatedAt.After(items[j].UpdatedAt)
			}
			return items[i].ID < items[j].ID
		})
		page = FrontierPage{
			SchemaVersion:   domain.SchemaVersionV1,
			MissionID:       missionID,
			MissionRevision: active.ID,
			Total:           len(items),
			Limit:           limit,
			Offset:          offset,
			StatusFilter:    filter.Status,
			FamilyFilter:    filter.Family,
			PolicyVersion:   policy.Version,
			MaxCandidates:   policy.MaxCandidates,
			MaxDepth:        policy.MaxDepth,
			Items:           sliceOpportunityPage(items, offset, limit),
		}
		return nil
	})
	if err != nil {
		return FrontierPage{}, err
	}
	return page, nil
}

// OpportunityInspector projects one opportunity with lineage counts.
func (p *Projector) OpportunityInspector(ctx context.Context, opportunityID domain.WorkOpportunityID) (WorkOpportunityDetail, error) {
	if opportunityID == "" {
		return WorkOpportunityDetail{}, errors.New("opportunity ID is required")
	}
	var detail WorkOpportunityDetail
	err := p.Store.View(ctx, func(r port.Reader) error {
		opp, err := r.WorkOpportunity(opportunityID)
		if err != nil {
			return err
		}
		policy, err := resolveHorizonPolicy(r)
		if err != nil {
			return err
		}
		all, err := r.WorkOpportunities(opp.MissionRevision, "")
		if err != nil {
			return err
		}
		children := 0
		siblings := 0
		sigPeers := 0
		for _, other := range all {
			if other.ID == opp.ID {
				continue
			}
			if other.ParentID == opp.ID {
				children++
			}
			if opp.ParentID != "" && other.ParentID == opp.ParentID {
				siblings++
			}
			if other.DedupSignature == opp.DedupSignature {
				sigPeers++
			}
		}
		summary, report := redactOpportunitySummary(summarizeOpportunity(opp, policy.MaxDepth))
		canSpawn := false
		spawnBlocker := ""
		if err := opp.CanSpawnChild(policy, children); err != nil {
			spawnBlocker = err.Error()
		} else {
			canSpawn = true
		}
		detail = WorkOpportunityDetail{
			SchemaVersion:  domain.SchemaVersionV1,
			Opportunity:    summary,
			PolicyVersion:  policy.Version,
			MaxDepth:       policy.MaxDepth,
			MaxCandidates:  policy.MaxCandidates,
			ChildrenCount:  children,
			SiblingCount:   siblings,
			SignaturePeers: sigPeers,
			CanSpawnChild:  canSpawn,
			SpawnBlocker:   spawnBlocker,
			Redaction:      report,
		}
		return nil
	})
	if err != nil {
		return WorkOpportunityDetail{}, err
	}
	return detail, nil
}

// FrontierHygieneForMission dry-runs reservoir hygiene for the active revision.
func (p *Projector) FrontierHygieneForMission(ctx context.Context, missionID domain.MissionID) (FrontierHygieneProjection, error) {
	if missionID == "" {
		return FrontierHygieneProjection{}, errors.New("mission ID is required")
	}
	now := p.Clock().UTC()
	var proj FrontierHygieneProjection
	err := p.Store.View(ctx, func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(missionID)
		if err != nil {
			return err
		}
		policy, err := resolveHorizonPolicy(r)
		if err != nil {
			return err
		}
		open, err := r.WorkOpportunities(active.ID, domain.OpportunityOpen)
		if err != nil {
			return err
		}
		deferred, err := r.WorkOpportunities(active.ID, domain.OpportunityDeferred)
		if err != nil {
			return err
		}
		unique, dupGroups, overDepth := reservoirSignatureStats(open, deferred, policy.MaxDepth)
		actions, err := domain.PlanFrontierReservoirHygiene(open, deferred, policy, now)
		if err != nil {
			return err
		}
		byID := map[domain.WorkOpportunityID]domain.WorkOpportunity{}
		for _, opp := range open {
			byID[opp.ID] = opp
		}
		for _, opp := range deferred {
			byID[opp.ID] = opp
		}
		summaries := make([]FrontierHygieneActionSummary, 0, len(actions))
		var deferredN, abandonedN, supersededN, reopenedN int
		for _, action := range actions {
			switch action.Transition.Event {
			case domain.OppEventDefer:
				deferredN++
			case domain.OppEventAbandon:
				abandonedN++
			case domain.OppEventSupersede:
				supersededN++
			case domain.OppEventReopen:
				reopenedN++
			}
			row := FrontierHygieneActionSummary{
				OpportunityID: action.OpportunityID,
				Event:         action.Transition.Event,
				Reason:        action.Transition.Reason,
				SupersededBy:  action.Transition.SupersededBy,
			}
			if opp, ok := byID[action.OpportunityID]; ok {
				row.Family = opp.Family
				row.Priority = opp.Priority
				row.Depth = opp.Depth
				row.StatusBefore = opp.Status
				row.DedupSignature = opp.DedupSignature
			}
			// Presentation redaction of free-text reason.
			if row.Reason != "" {
				text, _ := RedactSensitiveText(row.Reason)
				row.Reason, _ = BoundUTF8(text, opportunityTextMax)
			}
			summaries = append(summaries, row)
		}
		truncated := 0
		if len(summaries) > MaxFrontierHygieneActions {
			truncated = len(summaries) - MaxFrontierHygieneActions
			summaries = summaries[:MaxFrontierHygieneActions]
		}
		findings := []string{
			fmt.Sprintf("frontier:open_before=%d", len(open)),
			fmt.Sprintf("frontier:deferred_before=%d", len(deferred)),
			fmt.Sprintf("frontier:unique_signatures=%d", unique),
			fmt.Sprintf("frontier:duplicate_signature_groups=%d", dupGroups),
			fmt.Sprintf("frontier:over_depth_open=%d", overDepth),
			fmt.Sprintf("frontier:hygiene_actions=%d", len(actions)),
			fmt.Sprintf("frontier:hygiene_deferred=%d", deferredN),
			fmt.Sprintf("frontier:hygiene_abandoned=%d", abandonedN),
			fmt.Sprintf("frontier:hygiene_superseded=%d", supersededN),
			fmt.Sprintf("frontier:hygiene_reopened=%d", reopenedN),
		}
		if len(actions) == 0 {
			findings = append(findings, "frontier:hygiene_noop")
		}
		proj = FrontierHygieneProjection{
			SchemaVersion:    domain.SchemaVersionV1,
			MissionID:        missionID,
			MissionRevision:  active.ID,
			ObservedAt:       now,
			PolicyVersion:    policy.Version,
			MaxCandidates:    policy.MaxCandidates,
			MaxDepth:         policy.MaxDepth,
			OpenBefore:       len(open),
			DeferredBefore:   len(deferred),
			UniqueSignatures: unique,
			DuplicateGroups:  dupGroups,
			OverDepthOpen:    overDepth,
			NeedsCompact:     len(actions) > 0,
			ActionCount:      len(actions),
			ActionsTruncated: truncated,
			DeferredCount:    deferredN,
			AbandonedCount:   abandonedN,
			SupersededCount:  supersededN,
			ReopenedCount:    reopenedN,
			Actions:          summaries,
			Findings:         findings,
		}
		return nil
	})
	if err != nil {
		return FrontierHygieneProjection{}, err
	}
	return proj, nil
}

func resolveHorizonPolicy(r port.Reader) (domain.HorizonPolicy, error) {
	policy := domain.DefaultHorizonPolicy()
	if rev, err := r.ActiveConfigRevision(domain.ConfigScopeHorizon); err == nil && rev.Horizon != nil {
		if err := rev.Horizon.Validate(); err == nil {
			policy = *rev.Horizon
		}
	} else if err != nil && !errors.Is(err, port.ErrNotFound) {
		return domain.HorizonPolicy{}, err
	}
	return policy, nil
}

func summarizeOpportunity(opp domain.WorkOpportunity, maxDepth int) WorkOpportunitySummary {
	return WorkOpportunitySummary{
		ID:              opp.ID,
		MissionRevision: opp.MissionRevision,
		Family:          opp.Family,
		Status:          opp.Status,
		Title:           opp.Title,
		Origin:          opp.Origin,
		ExpectedGain:    opp.ExpectedGain,
		Novelty:         opp.Novelty,
		StopCondition:   opp.StopCondition,
		DedupSignature:  opp.DedupSignature,
		ParentID:        opp.ParentID,
		Depth:           opp.Depth,
		Priority:        opp.Priority,
		Risk:            opp.Risk,
		CreatedAt:       opp.CreatedAt.UTC(),
		UpdatedAt:       opp.UpdatedAt.UTC(),
		AdmittedInquiry: opp.AdmittedInquiryID,
		AbandonReason:   opp.AbandonReason,
		OverDepth:       opp.Status == domain.OpportunityOpen && opp.Depth > maxDepth,
	}
}

func redactOpportunitySummary(in WorkOpportunitySummary) (WorkOpportunitySummary, RedactionReport) {
	report := RedactionReport{}
	fields := []*string{&in.Title, &in.Origin, &in.ExpectedGain, &in.Novelty, &in.StopCondition, &in.DedupSignature, &in.AbandonReason}
	for _, field := range fields {
		if *field == "" {
			continue
		}
		text, matches := RedactSensitiveText(*field)
		if matches > 0 {
			report.Applied = true
			report.SecretMatches += matches
		}
		bound, removed := BoundUTF8(text, opportunityTextMax)
		if removed > 0 {
			report.Applied = true
			report.TruncatedBytes += removed
		}
		*field = bound
	}
	if report.Applied && len(report.Notes) == 0 {
		report.Notes = []string{"presentation redaction only; store unchanged"}
	}
	return in, report
}

func reservoirSignatureStats(open, deferred []domain.WorkOpportunity, maxDepth int) (unique, dupGroups, overDepth int) {
	sigs := map[string]int{}
	for _, opp := range open {
		sigs[opp.DedupSignature]++
		if opp.Depth > maxDepth {
			overDepth++
		}
	}
	for _, opp := range deferred {
		sigs[opp.DedupSignature]++
	}
	unique = len(sigs)
	for _, n := range sigs {
		if n > 1 {
			dupGroups++
		}
	}
	return unique, dupGroups, overDepth
}

func normalizeFrontierPage(limit, offset int) (int, int, error) {
	if limit <= 0 {
		limit = DefaultFrontierListLimit
	}
	if limit > MaxFrontierListLimit {
		return 0, 0, fmt.Errorf("limit must be between 1 and %d", MaxFrontierListLimit)
	}
	if offset < 0 {
		return 0, 0, errors.New("offset must be a non-negative integer")
	}
	return limit, offset, nil
}

func sliceOpportunityPage(items []WorkOpportunitySummary, offset, limit int) []WorkOpportunitySummary {
	if offset >= len(items) {
		return []WorkOpportunitySummary{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	out := make([]WorkOpportunitySummary, end-offset)
	copy(out, items[offset:end])
	return out
}

func parseWorkOpportunityStatus(raw string) (domain.WorkOpportunityStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	status := domain.WorkOpportunityStatus(strings.ToUpper(raw))
	if !status.Valid() {
		return "", fmt.Errorf("unknown work opportunity status filter %q", raw)
	}
	return status, nil
}

func parseWorkFamily(raw string) (domain.WorkFamily, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	family := domain.WorkFamily(raw)
	if !family.Valid() {
		return "", fmt.Errorf("unknown work family filter %q", raw)
	}
	return family, nil
}
