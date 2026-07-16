package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// UserAmendment is an explicit operator/user proposal for a new MissionRevision
// (FR-AUTH-004). It never mutates the active revision in place.
type UserAmendment struct {
	SchemaVersion int       `json:"schema_version"`
	MissionID     MissionID `json:"mission_id"`
	// BaseRevision is the numeric MissionRevision.Revision currently active.
	BaseRevision uint64 `json:"base_revision"`
	// CandidateRevision must equal BaseRevision+1; monotonic lineage only.
	CandidateRevision uint64        `json:"candidate_revision"`
	OriginalText      string        `json:"original_text"`
	Purpose           string        `json:"purpose"`
	Domains           []string      `json:"domains"`
	Policies          []string      `json:"policies"`
	Budget            Budget        `json:"budget"`
	Status            MissionStatus `json:"status"`
	Reason            string        `json:"reason"`
}

func (a UserAmendment) Validate() error {
	if a.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported user amendment schema version %d", a.SchemaVersion)
	}
	if a.MissionID == "" || a.BaseRevision == 0 || a.CandidateRevision == 0 {
		return errors.New("user amendment is missing mission_id or revision lineage")
	}
	if a.CandidateRevision != a.BaseRevision+1 {
		return fmt.Errorf("candidate revision %d must be base %d + 1", a.CandidateRevision, a.BaseRevision)
	}
	if strings.TrimSpace(a.OriginalText) == "" || strings.TrimSpace(a.Purpose) == "" || strings.TrimSpace(a.Reason) == "" {
		return errors.New("user amendment requires original_text, purpose, and reason")
	}
	if err := validateMissionStringSet("domains", a.Domains); err != nil {
		return err
	}
	if err := validateMissionStringSet("policies", a.Policies); err != nil {
		return err
	}
	switch a.Status {
	case MissionActive, MissionPaused, MissionCancelled:
	default:
		return fmt.Errorf("unknown mission status %q", a.Status)
	}
	return a.Budget.Validate()
}

func validateMissionStringSet(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("%s contains an empty value", name)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s contains duplicate %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

// MissionFieldChange is one deterministic path in a mission semantic diff.
type MissionFieldChange struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// MissionDiff is a pure field-level comparison between active and candidate.
type MissionDiff struct {
	MissionID         MissionID            `json:"mission_id"`
	BaseRevision      uint64               `json:"base_revision"`
	CandidateRevision uint64               `json:"candidate_revision"`
	Changes           []MissionFieldChange `json:"changes"`
	Empty             bool                 `json:"empty"`
}

func (d MissionDiff) Validate() error {
	if d.MissionID == "" || d.BaseRevision == 0 || d.CandidateRevision == 0 {
		return errors.New("mission diff is missing mission_id or revision lineage")
	}
	if d.CandidateRevision != d.BaseRevision+1 {
		return fmt.Errorf("mission diff candidate revision %d must be base %d + 1", d.CandidateRevision, d.BaseRevision)
	}
	if d.Empty && len(d.Changes) != 0 {
		return errors.New("empty mission diff must not list changes")
	}
	if !d.Empty && len(d.Changes) == 0 {
		return errors.New("non-empty mission diff requires changes")
	}
	for i := 1; i < len(d.Changes); i++ {
		if d.Changes[i-1].Path >= d.Changes[i].Path {
			return errors.New("mission diff changes must be strictly sorted by path")
		}
	}
	return nil
}

// MissionImpactDisposition classifies how an entity class is affected.
type MissionImpactDisposition string

const (
	ImpactKeep          MissionImpactDisposition = "KEEP"
	ImpactInvalidate    MissionImpactDisposition = "INVALIDATE"
	ImpactCancel        MissionImpactDisposition = "CANCEL"
	ImpactReprioritize  MissionImpactDisposition = "REPRIORITIZE"
	ImpactNewCapability MissionImpactDisposition = "NEW_SCOPE"
)

func (d MissionImpactDisposition) Valid() bool {
	switch d {
	case ImpactKeep, ImpactInvalidate, ImpactCancel, ImpactReprioritize, ImpactNewCapability:
		return true
	default:
		return false
	}
}

// MissionImpactItem is one pure impact classification before acceptance.
type MissionImpactItem struct {
	Kind        string                   `json:"kind"`
	Disposition MissionImpactDisposition `json:"disposition"`
	Reason      string                   `json:"reason"`
	Reference   string                   `json:"reference,omitempty"`
}

// MissionImpactPreview is the pure analysis required by FR-AUTH-004.
type MissionImpactPreview struct {
	MissionID          MissionID           `json:"mission_id"`
	BaseRevision       uint64              `json:"base_revision"`
	CandidateRevision  uint64              `json:"candidate_revision"`
	Items              []MissionImpactItem `json:"items"`
	RequiresAcceptance bool                `json:"requires_acceptance"`
	Blocked            bool                `json:"blocked"`
	Notes              []string            `json:"notes,omitempty"`
}

func (p MissionImpactPreview) Validate() error {
	if p.MissionID == "" || p.BaseRevision == 0 || p.CandidateRevision == 0 {
		return errors.New("mission impact is missing mission_id or revision lineage")
	}
	if p.CandidateRevision != p.BaseRevision+1 {
		return fmt.Errorf("mission impact candidate revision %d must be base %d + 1", p.CandidateRevision, p.BaseRevision)
	}
	if p.Blocked && p.RequiresAcceptance {
		return errors.New("blocked mission impact must not require acceptance")
	}
	for _, item := range p.Items {
		if strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.Reason) == "" || !item.Disposition.Valid() {
			return errors.New("mission impact item is incomplete")
		}
	}
	return nil
}

// CandidateFromAmendment builds an ephemeral MissionRevision-shaped candidate
// for pure diff/impact. Persistence still assigns ID, Provenance, AcceptedAt.
func CandidateFromAmendment(base MissionRevision, amendment UserAmendment) (MissionRevision, error) {
	if err := base.Validate(); err != nil {
		return MissionRevision{}, fmt.Errorf("validate base mission revision: %w", err)
	}
	if err := amendment.Validate(); err != nil {
		return MissionRevision{}, fmt.Errorf("validate user amendment: %w", err)
	}
	if amendment.MissionID != base.MissionID {
		return MissionRevision{}, errors.New("amendment mission_id disagrees with base")
	}
	if amendment.BaseRevision != base.Revision {
		return MissionRevision{}, fmt.Errorf("amendment base_revision %d disagrees with active revision %d", amendment.BaseRevision, base.Revision)
	}
	candidate := MissionRevision{
		SchemaVersion: SchemaVersionV1,
		ID:            "candidate",
		MissionID:     amendment.MissionID,
		Revision:      amendment.CandidateRevision,
		OriginalText:  amendment.OriginalText,
		Purpose:       amendment.Purpose,
		Domains:       append([]string(nil), amendment.Domains...),
		Policies:      append([]string(nil), amendment.Policies...),
		Budget:        amendment.Budget,
		Status:        amendment.Status,
		Provenance:    "candidate",
		AcceptedAt:    base.AcceptedAt,
	}
	if err := candidate.Validate(); err != nil {
		return MissionRevision{}, fmt.Errorf("build candidate mission revision: %w", err)
	}
	return candidate, nil
}

// DiffMissionRevisions compares base (active) and candidate content fields.
// Identity fields (id, provenance, accepted_at) are ignored.
func DiffMissionRevisions(base, candidate MissionRevision) (MissionDiff, error) {
	if err := base.Validate(); err != nil {
		return MissionDiff{}, fmt.Errorf("validate base mission revision: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return MissionDiff{}, fmt.Errorf("validate candidate mission revision: %w", err)
	}
	if base.MissionID != candidate.MissionID {
		return MissionDiff{}, errors.New("mission ids disagree")
	}
	if candidate.Revision != base.Revision+1 {
		return MissionDiff{}, fmt.Errorf("candidate revision %d must be base %d + 1", candidate.Revision, base.Revision)
	}

	before := map[string]string{}
	after := map[string]string{}
	collectMissionFields(before, base)
	collectMissionFields(after, candidate)

	paths := make(map[string]struct{}, len(before)+len(after))
	for p := range before {
		paths[p] = struct{}{}
	}
	for p := range after {
		paths[p] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)

	changes := make([]MissionFieldChange, 0)
	for _, path := range ordered {
		b, a := before[path], after[path]
		if b == a {
			continue
		}
		changes = append(changes, MissionFieldChange{Path: path, Before: b, After: a})
	}
	diff := MissionDiff{
		MissionID:         base.MissionID,
		BaseRevision:      base.Revision,
		CandidateRevision: candidate.Revision,
		Changes:           changes,
		Empty:             len(changes) == 0,
	}
	if err := diff.Validate(); err != nil {
		return MissionDiff{}, err
	}
	return diff, nil
}

func collectMissionFields(dst map[string]string, m MissionRevision) {
	dst["original_text"] = m.OriginalText
	dst["purpose"] = m.Purpose
	dst["status"] = string(m.Status)
	dst["budget.model_calls"] = fmt.Sprintf("%d", m.Budget.ModelCalls)
	dst["budget.tokens"] = fmt.Sprintf("%d", m.Budget.Tokens)
	dst["budget.bytes"] = fmt.Sprintf("%d", m.Budget.Bytes)
	dst["budget.attempts"] = fmt.Sprintf("%d", m.Budget.Attempts)
	dst["budget.duration"] = m.Budget.Duration.String()
	dst["domains"] = joinSortedCopy(m.Domains)
	dst["policies"] = joinSortedCopy(m.Policies)
}

func joinSortedCopy(values []string) string {
	cp := append([]string(nil), values...)
	sort.Strings(cp)
	return strings.Join(cp, "\n")
}

// PreviewMissionImpact derives agenda/knowledge consequences from a validated
// diff. Empty diffs are blocked no-ops; acceptance is still required for any
// non-empty change before agenda reconciliation may run.
func PreviewMissionImpact(base, candidate MissionRevision, diff MissionDiff) (MissionImpactPreview, error) {
	if err := base.Validate(); err != nil {
		return MissionImpactPreview{}, err
	}
	if err := candidate.Validate(); err != nil {
		return MissionImpactPreview{}, err
	}
	if err := diff.Validate(); err != nil {
		return MissionImpactPreview{}, err
	}
	if base.MissionID != candidate.MissionID || base.MissionID != diff.MissionID {
		return MissionImpactPreview{}, errors.New("impact mission ids disagree")
	}
	if diff.BaseRevision != base.Revision || diff.CandidateRevision != candidate.Revision {
		return MissionImpactPreview{}, errors.New("impact revision lineage disagrees with revisions")
	}

	preview := MissionImpactPreview{
		MissionID:         base.MissionID,
		BaseRevision:      base.Revision,
		CandidateRevision: candidate.Revision,
	}
	if diff.Empty {
		preview.Blocked = true
		preview.Notes = append(preview.Notes, "no-op amendment has no field changes")
		if err := preview.Validate(); err != nil {
			return MissionImpactPreview{}, err
		}
		return preview, nil
	}

	preview.RequiresAcceptance = true
	changed := map[string]MissionFieldChange{}
	for _, c := range diff.Changes {
		changed[c.Path] = c
	}

	// Prior knowledge remains versioned; never auto-deleted on mission amend.
	preview.Items = append(preview.Items, MissionImpactItem{
		Kind:        "knowledge",
		Disposition: ImpactKeep,
		Reason:      "prior knowledge remains versioned under previous mission revision",
		Reference:   "claims_observations_artifacts",
	})

	if candidate.Status == MissionCancelled || changed["status"].After == string(MissionCancelled) {
		preview.Items = append(preview.Items, MissionImpactItem{
			Kind:        "agenda",
			Disposition: ImpactCancel,
			Reason:      "candidate mission status cancels non-terminal agenda units of the previous revision",
			Reference:   "operations_inquiries",
		})
	} else {
		if _, ok := changed["purpose"]; ok {
			preview.Items = append(preview.Items, MissionImpactItem{
				Kind:        "agenda",
				Disposition: ImpactInvalidate,
				Reason:      "purpose change requires re-evaluation of open inquiries admitted under the prior purpose",
				Reference:   "purpose",
			})
		}
		if _, ok := changed["domains"]; ok {
			removed, added := setDiff(base.Domains, candidate.Domains)
			if len(removed) > 0 {
				preview.Items = append(preview.Items, MissionImpactItem{
					Kind:        "agenda",
					Disposition: ImpactInvalidate,
					Reason:      "removed domains invalidate work scoped only to those domains",
					Reference:   "domains.removed:" + strings.Join(removed, ","),
				})
			}
			if len(added) > 0 {
				preview.Items = append(preview.Items, MissionImpactItem{
					Kind:        "agenda",
					Disposition: ImpactNewCapability,
					Reason:      "added domains open new inquiry scope without cancelling prior units",
					Reference:   "domains.added:" + strings.Join(added, ","),
				})
			}
		}
		if _, ok := changed["policies"]; ok {
			preview.Items = append(preview.Items, MissionImpactItem{
				Kind:        "agenda",
				Disposition: ImpactInvalidate,
				Reason:      "policy change requires revalidation of open work against new constraints",
				Reference:   "policies",
			})
		}
		if budgetReduced(base.Budget, candidate.Budget) {
			preview.Items = append(preview.Items, MissionImpactItem{
				Kind:        "agenda",
				Disposition: ImpactReprioritize,
				Reason:      "reduced budget forces reprioritization of open work",
				Reference:   "budget",
			})
		}
		if _, ok := changed["original_text"]; ok {
			preview.Items = append(preview.Items, MissionImpactItem{
				Kind:        "mission_text",
				Disposition: ImpactKeep,
				Reason:      "original text is preserved per revision; prior text remains on the previous revision",
				Reference:   "original_text",
			})
		}
		// Default: non-terminal units of the superseded revision are cancelled so
		// the new active revision starts with a clean scheduling surface. New
		// work must be admitted against the new revision.
		preview.Items = append(preview.Items, MissionImpactItem{
			Kind:        "agenda",
			Disposition: ImpactCancel,
			Reason:      "acceptance cancels non-terminal operations and inquiries bound to the superseded revision",
			Reference:   "previous_revision_units",
		})
		preview.Items = append(preview.Items, MissionImpactItem{
			Kind:        "frontier",
			Disposition: ImpactCancel,
			Reason:      "acceptance abandons open/deferred work opportunities of the superseded revision",
			Reference:   "work_opportunities",
		})
	}

	sort.SliceStable(preview.Items, func(i, j int) bool {
		if preview.Items[i].Kind != preview.Items[j].Kind {
			return preview.Items[i].Kind < preview.Items[j].Kind
		}
		if preview.Items[i].Disposition != preview.Items[j].Disposition {
			return preview.Items[i].Disposition < preview.Items[j].Disposition
		}
		return preview.Items[i].Reference < preview.Items[j].Reference
	})
	if err := preview.Validate(); err != nil {
		return MissionImpactPreview{}, err
	}
	return preview, nil
}

func budgetReduced(before, after Budget) bool {
	return after.ModelCalls < before.ModelCalls ||
		after.Tokens < before.Tokens ||
		after.Bytes < before.Bytes ||
		after.Attempts < before.Attempts ||
		after.Duration < before.Duration
}

func setDiff(before, after []string) (removed, added []string) {
	b := map[string]struct{}{}
	a := map[string]struct{}{}
	for _, v := range before {
		b[strings.TrimSpace(v)] = struct{}{}
	}
	for _, v := range after {
		a[strings.TrimSpace(v)] = struct{}{}
	}
	for v := range b {
		if _, ok := a[v]; !ok {
			removed = append(removed, v)
		}
	}
	for v := range a {
		if _, ok := b[v]; !ok {
			added = append(added, v)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	return removed, added
}

// AgendaReconciliationReport records concrete units cancelled during acceptance.
type AgendaReconciliationReport struct {
	PreviousRevision       MissionRevisionID   `json:"previous_revision_id"`
	NewRevision            MissionRevisionID   `json:"new_revision_id"`
	CancelledOperations    []OperationID       `json:"cancelled_operations"`
	CancelledInquiries     []InquiryID         `json:"cancelled_inquiries"`
	AbandonedOpportunities []WorkOpportunityID `json:"abandoned_opportunities"`
}

func (r AgendaReconciliationReport) Validate() error {
	if r.PreviousRevision == "" || r.NewRevision == "" {
		return errors.New("agenda reconciliation requires previous and new revision ids")
	}
	return nil
}
