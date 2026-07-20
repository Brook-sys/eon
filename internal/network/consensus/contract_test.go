package consensus_test

import (
	"testing"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/network/consensus"
)

func TestClaimProposal_Skeleton(t *testing.T) {
	proposal := consensus.ClaimProposal{
		ProposalID: "p-123",
		ProposerID: "node-a",
		Claim: domain.Claim{
			ID: "claim-1",
		},
	}
	if proposal.ProposalID != "p-123" {
		t.Errorf("expected proposal ID p-123, got %s", proposal.ProposalID)
	}
}
