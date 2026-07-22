package consensus

import (
	"context"
	"fmt"
	"motor-autonomo/internal/domain"
	"sync"
)

type MemoryConsensusNode struct {
	mu        sync.RWMutex
	nodeID    string
	quorum    int
	proposals map[string]*ClaimProposal
	votes     map[string][]ClaimVote
	committed map[string]bool
}

func NewMemoryConsensusNode(nodeID string, quorum int) *MemoryConsensusNode {
	return &MemoryConsensusNode{
		nodeID:    nodeID,
		quorum:    quorum,
		proposals: make(map[string]*ClaimProposal),
		votes:     make(map[string][]ClaimVote),
		committed: make(map[string]bool),
	}
}

func (n *MemoryConsensusNode) Propose(ctx context.Context, claim domain.Claim) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if claim.ID == "" {
		return "", fmt.Errorf("claim ID cannot be empty")
	}

	proposalID := "prop-" + string(claim.ID)
	if _, exists := n.proposals[proposalID]; exists {
		return "", fmt.Errorf("proposal already exists")
	}

	n.proposals[proposalID] = &ClaimProposal{
		ProposalID: proposalID,
		ProposerID: n.nodeID,
		Claim:      claim,
	}

	// Self-vote
	n.votes[proposalID] = []ClaimVote{
		{
			ProposalID: proposalID,
			VoterID:    n.nodeID,
			Decision:   VoteAccept,
			Reason:     "self-proposed",
		},
	}

	return proposalID, nil
}

func (n *MemoryConsensusNode) CastVote(ctx context.Context, proposalID string, decision Vote, reason string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, exists := n.proposals[proposalID]; !exists {
		return fmt.Errorf("proposal not found")
	}

	// Prevent duplicate voting from same node
	for _, v := range n.votes[proposalID] {
		if v.VoterID == n.nodeID {
			return fmt.Errorf("node already voted on this proposal")
		}
	}

	n.votes[proposalID] = append(n.votes[proposalID], ClaimVote{
		ProposalID: proposalID,
		VoterID:    n.nodeID,
		Decision:   decision,
		Reason:     reason,
	})

	return nil
}

func (n *MemoryConsensusNode) Verify(ctx context.Context, proposalID string) (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, exists := n.proposals[proposalID]; !exists {
		return false, fmt.Errorf("proposal not found")
	}

	accepts := 0
	for _, v := range n.votes[proposalID] {
		if v.Decision == VoteAccept {
			accepts++
		}
	}

	if accepts >= n.quorum {
		n.committed[proposalID] = true
		return true, nil
	}

	return false, nil
}

// Internal method for tests to simulate remote peer vote
func (n *MemoryConsensusNode) InjectRemoteVote(proposalID string, peerID string, decision Vote, reason string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.votes[proposalID] = append(n.votes[proposalID], ClaimVote{
		ProposalID: proposalID,
		VoterID:    peerID,
		Decision:   decision,
		Reason:     reason,
	})
}
