// Package domain defines the persistence-facing vocabulary of the epistemic
// runtime. It deliberately depends only on the Go standard library and does
// not import storage, provider, or transport adapters.
package domain

// Distinct ID types prevent accidental cross-entity assignment while keeping
// persisted values backend-independent.
type (
	MissionID             string
	MissionRevisionID     string
	QuestionID            string
	InquiryCandidateID    string
	InquiryID             string
	OperationSpecID       string
	OperationID           string
	SourceID              string
	SourceVersionID       string
	SourceFragmentID      string
	ObservationID         string
	ClaimID               string
	EvidenceLinkID        string
	ArtifactID            string
	ChangeSetID           string
	CommitID              string
	FailureID             string
	EventID               string
	ReceiptID             string
	CommandID             string
	ExternalEventID       string
	OperatorQuestionID    string
	OperatorAnswerID      string
	WorkOpportunityID     string
	ContinuityDiagnosisID string
	IdempotencyKey        string
)
