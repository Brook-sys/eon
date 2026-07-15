package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const GenesisCommitID CommitID = "commit_genesis"

type Source struct {
	SchemaVersion int       `json:"schema_version"`
	ID            SourceID  `json:"id"`
	Kind          string    `json:"kind"`
	Locator       string    `json:"locator"`
	ObservedAt    time.Time `json:"observed_at"`
}

func (s Source) Validate() error {
	if s.SchemaVersion != SchemaVersionV1 || s.ID == "" || strings.TrimSpace(s.Kind) == "" || strings.TrimSpace(s.Locator) == "" || s.ObservedAt.IsZero() {
		return errors.New("source is incomplete or has unsupported schema version")
	}
	return nil
}

type SourceVersion struct {
	SchemaVersion   int             `json:"schema_version"`
	ID              SourceVersionID `json:"id"`
	SourceID        SourceID        `json:"source_id"`
	ContentHash     string          `json:"content_hash"`
	ContentRef      string          `json:"content_ref"`
	ExternalVersion string          `json:"external_version,omitempty"`
	ObservedAt      time.Time       `json:"observed_at"`
}

func (v SourceVersion) Validate() error {
	if v.SchemaVersion != SchemaVersionV1 || v.ID == "" || v.SourceID == "" || strings.TrimSpace(v.ContentHash) == "" || strings.TrimSpace(v.ContentRef) == "" || v.ObservedAt.IsZero() {
		return errors.New("source version is incomplete or has unsupported schema version")
	}
	return nil
}

// SourceSnapshot preserves the exact immutable bytes used to derive a source
// version. Large backends may place Content in an artifact store while keeping
// the same content-addressed reference and hash contract.
type SourceSnapshot struct {
	SchemaVersion   int             `json:"schema_version"`
	SourceVersionID SourceVersionID `json:"source_version_id"`
	MediaType       string          `json:"media_type"`
	Content         []byte          `json:"content"`
}

func (s SourceSnapshot) Validate() error {
	if s.SchemaVersion != SchemaVersionV1 || s.SourceVersionID == "" || strings.TrimSpace(s.MediaType) == "" || len(s.Content) == 0 {
		return errors.New("source snapshot is incomplete or has unsupported schema version")
	}
	return nil
}

type SourceFragment struct {
	SchemaVersion   int              `json:"schema_version"`
	ID              SourceFragmentID `json:"id"`
	SourceVersionID SourceVersionID  `json:"source_version_id"`
	Location        string           `json:"location"`
	StartOffset     uint64           `json:"start_offset"`
	EndOffset       uint64           `json:"end_offset"`
	ContentHash     string           `json:"content_hash"`
	ContentRef      string           `json:"content_ref"`
}

func (f SourceFragment) Validate() error {
	if f.SchemaVersion != SchemaVersionV1 || f.ID == "" || f.SourceVersionID == "" || strings.TrimSpace(f.Location) == "" || f.EndOffset <= f.StartOffset || strings.TrimSpace(f.ContentHash) == "" || strings.TrimSpace(f.ContentRef) == "" {
		return errors.New("source fragment is incomplete or has unsupported schema version")
	}
	return nil
}

// ObservationAnchor distinguishes epistemic source material from operational
// receipts. Exactly one side must be present (FR-KNOW-001, INV-SAFE-004).
type ObservationAnchor struct {
	SourceFragmentID SourceFragmentID `json:"source_fragment_id,omitempty"`
	ReceiptID        ReceiptID        `json:"receipt_id,omitempty"`
}

func (a ObservationAnchor) Validate() error {
	hasFragment := a.SourceFragmentID != ""
	hasReceipt := a.ReceiptID != ""
	if hasFragment == hasReceipt {
		return errors.New("observation must have exactly one source fragment or evidence receipt anchor")
	}
	return nil
}

type Observation struct {
	SchemaVersion int               `json:"schema_version"`
	ID            ObservationID     `json:"id"`
	Statement     string            `json:"statement"`
	Anchor        ObservationAnchor `json:"anchor"`
	Provenance    string            `json:"provenance"`
}

func (o Observation) Validate() error {
	if o.SchemaVersion != SchemaVersionV1 || o.ID == "" || o.Statement == "" || o.Provenance == "" {
		return errors.New("observation is incomplete or has unsupported schema version")
	}
	return o.Anchor.Validate()
}

type Claim struct {
	SchemaVersion int               `json:"schema_version"`
	ID            ClaimID           `json:"id"`
	Proposition   string            `json:"proposition"`
	Qualifiers    map[string]string `json:"qualifiers"`
	Version       uint64            `json:"version"`
}

func (c Claim) Validate() error {
	if c.SchemaVersion != SchemaVersionV1 || c.ID == "" || c.Proposition == "" || c.Version == 0 {
		return errors.New("claim is incomplete or has unsupported schema version")
	}
	return nil
}

type EvidenceRelation string

const (
	EvidenceSupports         EvidenceRelation = "SUPPORTS"
	EvidenceContradicts      EvidenceRelation = "CONTRADICTS"
	EvidenceQualifies        EvidenceRelation = "QUALIFIES"
	EvidenceReplicates       EvidenceRelation = "REPLICATES"
	EvidenceFailsToReplicate EvidenceRelation = "FAILS_TO_REPLICATE"
	EvidenceDerivedFrom      EvidenceRelation = "DERIVED_FROM"
	EvidenceMentions         EvidenceRelation = "MENTIONS"
	EvidenceSupersedes       EvidenceRelation = "SUPERSEDES"
)

type EvidenceLink struct {
	SchemaVersion int              `json:"schema_version"`
	ID            EvidenceLinkID   `json:"id"`
	ObservationID ObservationID    `json:"observation_id"`
	ClaimID       ClaimID          `json:"claim_id"`
	Relation      EvidenceRelation `json:"relation"`
	Rationale     string           `json:"rationale,omitempty"`
}

func (e EvidenceLink) Validate() error {
	if e.SchemaVersion != SchemaVersionV1 || e.ID == "" || e.ObservationID == "" || e.ClaimID == "" {
		return errors.New("evidence link is incomplete or has unsupported schema version")
	}
	switch e.Relation {
	case EvidenceSupports, EvidenceContradicts, EvidenceQualifies, EvidenceReplicates,
		EvidenceFailsToReplicate, EvidenceDerivedFrom, EvidenceMentions, EvidenceSupersedes:
		return nil
	default:
		return fmt.Errorf("unknown evidence relation %q", e.Relation)
	}
}

type KnowledgeArtifact struct {
	SchemaVersion int        `json:"schema_version"`
	ID            ArtifactID `json:"id"`
	Kind          string     `json:"kind"`
	BaseCommitID  CommitID   `json:"base_commit_id"`
	Dependencies  []string   `json:"dependencies"`
	ContentRef    string     `json:"content_ref"`
	Stale         bool       `json:"stale"`
}

type ChangeKind string

const (
	ChangeAdd       ChangeKind = "ADD"
	ChangeReplace   ChangeKind = "REPLACE"
	ChangeDeprecate ChangeKind = "DEPRECATE"
	ChangeLink      ChangeKind = "LINK"
	ChangeUnlink    ChangeKind = "UNLINK"
)

type Change struct {
	Kind       ChangeKind `json:"kind"`
	EntityType string     `json:"entity_type"`
	EntityID   string     `json:"entity_id"`
	PayloadRef string     `json:"payload_ref"`
}

func (c Change) Validate() error {
	if strings.TrimSpace(c.EntityType) == "" || strings.TrimSpace(c.EntityID) == "" {
		return errors.New("change is missing entity type or entity ID")
	}
	switch c.Kind {
	case ChangeAdd, ChangeReplace:
		if strings.TrimSpace(c.PayloadRef) == "" {
			return errors.New("add and replace changes require a payload reference")
		}
	case ChangeDeprecate:
		if c.PayloadRef != "" {
			return errors.New("deprecate changes must not contain a payload reference")
		}
	case ChangeLink, ChangeUnlink:
		return fmt.Errorf("change kind %q is not supported by the first vertical slice", c.Kind)
	default:
		return fmt.Errorf("unknown change kind %q", c.Kind)
	}
	return nil
}

type ProposedChangeSet struct {
	SchemaVersion   int               `json:"schema_version"`
	ID              ChangeSetID       `json:"id"`
	MissionRevision MissionRevisionID `json:"mission_revision_id"`
	OperationID     OperationID       `json:"operation_id"`
	BaseCommitID    CommitID          `json:"base_commit_id"`
	ReadSet         []string          `json:"read_set"`
	Preconditions   []string          `json:"preconditions"`
	Changes         []Change          `json:"changes"`
	ExpectedDelta   string            `json:"expected_delta"`
	ValidatorIDs    []string          `json:"validator_ids"`
	Provenance      string            `json:"provenance"`
	IdempotencyKey  IdempotencyKey    `json:"idempotency_key"`
}

func (p ProposedChangeSet) Validate() error {
	if p.SchemaVersion != SchemaVersionV1 || p.ID == "" || p.MissionRevision == "" || p.OperationID == "" || p.BaseCommitID == "" || len(p.Changes) == 0 || p.ExpectedDelta == "" || len(p.ValidatorIDs) == 0 || p.Provenance == "" || p.IdempotencyKey == "" {
		return errors.New("proposed changeset is incomplete or has unsupported schema version")
	}
	targets := make(map[string]struct{}, len(p.Changes))
	for _, change := range p.Changes {
		if err := change.Validate(); err != nil {
			return err
		}
		key := change.EntityType + "\x00" + change.EntityID
		if _, duplicate := targets[key]; duplicate {
			return fmt.Errorf("changeset contains duplicate target %s/%s", change.EntityType, change.EntityID)
		}
		targets[key] = struct{}{}
	}
	if hasBlankOrDuplicate(p.ReadSet) || hasBlankOrDuplicate(p.Preconditions) || hasBlankOrDuplicate(p.ValidatorIDs) {
		return errors.New("changeset lists must not contain blank or duplicate values")
	}
	return nil
}

type AcceptedChangeSet struct {
	SchemaVersion        int         `json:"schema_version"`
	ID                   ChangeSetID `json:"id"`
	ProposedChangeSetID  ChangeSetID `json:"proposed_change_set_id"`
	ValidationReceiptIDs []ReceiptID `json:"validation_receipt_ids"`
	AcceptedAt           time.Time   `json:"accepted_at"`
	PolicyVersion        string      `json:"policy_version"`
}

func (a AcceptedChangeSet) Validate() error {
	if a.SchemaVersion != SchemaVersionV1 || a.ID == "" || a.ProposedChangeSetID == "" || len(a.ValidationReceiptIDs) == 0 || a.AcceptedAt.IsZero() || strings.TrimSpace(a.PolicyVersion) == "" {
		return errors.New("accepted changeset is incomplete or has unsupported schema version")
	}
	if hasBlankOrDuplicateReceipts(a.ValidationReceiptIDs) {
		return errors.New("accepted changeset receipt IDs must not be blank or duplicated")
	}
	return nil
}

type Commit struct {
	SchemaVersion       int               `json:"schema_version"`
	ID                  CommitID          `json:"id"`
	AcceptedChangeSetID ChangeSetID       `json:"accepted_change_set_id"`
	MissionRevision     MissionRevisionID `json:"mission_revision_id"`
	BaseCommitID        CommitID          `json:"base_commit_id"`
	Version             uint64            `json:"version"`
	CommittedAt         time.Time         `json:"committed_at"`
	ReceiptID           ReceiptID         `json:"receipt_id"`
	IdempotencyKey      IdempotencyKey    `json:"idempotency_key"`
}

func (c Commit) Validate() error {
	if c.SchemaVersion != SchemaVersionV1 || c.ID == "" || c.AcceptedChangeSetID == "" || c.MissionRevision == "" || c.BaseCommitID == "" || c.Version == 0 || c.CommittedAt.IsZero() || c.ReceiptID == "" || c.IdempotencyKey == "" {
		return errors.New("commit is incomplete or has unsupported schema version")
	}
	return nil
}

type CommitReceipt struct {
	SchemaVersion int         `json:"schema_version"`
	ID            ReceiptID   `json:"id"`
	CommitID      CommitID    `json:"commit_id"`
	ChangeSetID   ChangeSetID `json:"change_set_id"`
	OperationID   OperationID `json:"operation_id"`
	Version       uint64      `json:"version"`
	ProducedAt    time.Time   `json:"produced_at"`
}

func (r CommitReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || r.ID == "" || r.CommitID == "" || r.ChangeSetID == "" || r.OperationID == "" || r.Version == 0 || r.ProducedAt.IsZero() {
		return errors.New("commit receipt is incomplete or has unsupported schema version")
	}
	return nil
}

// RawModelOutput preserves the exact bounded provider text before parsing or
// validation. It is evidence, never canonical knowledge by itself.
type RawModelOutput struct {
	SchemaVersion int         `json:"schema_version"`
	ID            ArtifactID  `json:"id"`
	OperationID   OperationID `json:"operation_id"`
	Model         string      `json:"model"`
	Content       string      `json:"content"`
	ContentHash   string      `json:"content_hash"`
	CreatedAt     time.Time   `json:"created_at"`
}

func (o RawModelOutput) Validate() error {
	if o.SchemaVersion != SchemaVersionV1 || o.ID == "" || o.OperationID == "" || strings.TrimSpace(o.Model) == "" || o.Content == "" || o.ContentHash == "" || o.CreatedAt.IsZero() {
		return errors.New("raw model output is incomplete or has unsupported schema version")
	}
	return nil
}

type ValidationReceipt struct {
	SchemaVersion int         `json:"schema_version"`
	ID            ReceiptID   `json:"id"`
	OperationID   OperationID `json:"operation_id"`
	ChangeSetID   ChangeSetID `json:"change_set_id"`
	ValidatorID   string      `json:"validator_id"`
	Passed        bool        `json:"passed"`
	ArtifactRef   ArtifactID  `json:"artifact_ref"`
	ProducedAt    time.Time   `json:"produced_at"`
}

func (r ValidationReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || r.ID == "" || r.OperationID == "" || r.ChangeSetID == "" || strings.TrimSpace(r.ValidatorID) == "" || !r.Passed || r.ArtifactRef == "" || r.ProducedAt.IsZero() {
		return errors.New("validation receipt is incomplete, failed, or has unsupported schema version")
	}
	return nil
}

// CanonicalEntity is the backend-independent materialized head for one
// changeset target. PayloadRef addresses immutable content outside this map.
type CanonicalEntity struct {
	EntityType string   `json:"entity_type"`
	EntityID   string   `json:"entity_id"`
	PayloadRef string   `json:"payload_ref,omitempty"`
	Deprecated bool     `json:"deprecated"`
	Version    uint64   `json:"version"`
	CommitID   CommitID `json:"commit_id"`
}

func hasBlankOrDuplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func hasBlankOrDuplicateReceipts(values []ReceiptID) bool {
	seen := make(map[ReceiptID]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return true
		}
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
