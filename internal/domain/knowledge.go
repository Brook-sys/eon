package domain

import (
	"errors"
	"fmt"
	"time"
)

type Source struct {
	SchemaVersion int       `json:"schema_version"`
	ID            SourceID  `json:"id"`
	Kind          string    `json:"kind"`
	Locator       string    `json:"locator"`
	ObservedAt    time.Time `json:"observed_at"`
}

type SourceVersion struct {
	SchemaVersion   int             `json:"schema_version"`
	ID              SourceVersionID `json:"id"`
	SourceID        SourceID        `json:"source_id"`
	ContentHash     string          `json:"content_hash"`
	ExternalVersion string          `json:"external_version,omitempty"`
	ObservedAt      time.Time       `json:"observed_at"`
}

type SourceFragment struct {
	SchemaVersion   int              `json:"schema_version"`
	ID              SourceFragmentID `json:"id"`
	SourceVersionID SourceVersionID  `json:"source_version_id"`
	Location        string           `json:"location"`
	ContentHash     string           `json:"content_hash"`
	ContentRef      string           `json:"content_ref"`
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
