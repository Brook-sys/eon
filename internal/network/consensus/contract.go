package consensus

import (
	"context"
	"motor-autonomo/internal/domain"
)

type Vote uint8

const (
	VotePending Vote = iota
	VoteAccept
	VoteReject
)

// ClaimProposal represents a distributed proposal to accept a new claim into the shared knowledge base.
type ClaimProposal struct {
	ProposalID string
	ProposerID string
	Claim      domain.Claim
}

// ClaimVote represents a peer's decision on a pending ClaimProposal.
type ClaimVote struct {
	ProposalID string
	VoterID    string
	Decision   Vote
	Reason     string
}

// ConsensusContract defines the interface for distributed epistemological consensus over the P2P mesh.
type ConsensusContract interface {
	// Propose initiates a voting round for a new claim.
	Propose(ctx context.Context, claim domain.Claim) (string, error)
	// CastVote submits a local vote on a remote proposal.
	CastVote(ctx context.Context, proposalID string, decision Vote, reason string) error
	// Verify checks if a proposal has reached quorum and can be committed.
	Verify(ctx context.Context, proposalID string) (bool, error)
}
