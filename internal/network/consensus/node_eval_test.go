package consensus_test

import (
	"context"
	"testing"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/network/consensus"
)

func TestMemoryConsensusNode_ProposeAndVerify(t *testing.T) {
	node := consensus.NewMemoryConsensusNode("node-a", 2)
	
	claim := domain.Claim{
		ID: "claim-test-1",
	}
	
	// Node proposes (votes self automatically)
	propID, err := node.Propose(context.Background(), claim)
	if err != nil {
		t.Fatalf("failed to propose: %v", err)
	}
	
	// Should not have quorum yet (quorum is 2)
	ok, err := node.Verify(context.Background(), propID)
	if err != nil {
		t.Fatalf("failed to verify: %v", err)
	}
	if ok {
		t.Errorf("expected false, got true (quorum not reached)")
	}
	
	// Simulate remote peer accepting
	node.InjectRemoteVote(propID, "node-b", consensus.VoteAccept, "LGTM")
	
	// Now should have quorum
	ok, err = node.Verify(context.Background(), propID)
	if err != nil {
		t.Fatalf("failed to verify: %v", err)
	}
	if !ok {
		t.Errorf("expected true, got false (quorum reached)")
	}
}

func TestMemoryConsensusNode_DoubleVotingRejection(t *testing.T) {
	node := consensus.NewMemoryConsensusNode("node-a", 2)
	propID, _ := node.Propose(context.Background(), domain.Claim{ID: "claim-test-2"})
	
	err := node.CastVote(context.Background(), propID, consensus.VoteAccept, "another vote")
	if err == nil || err.Error() != "node already voted on this proposal" {
		t.Errorf("expected duplicate vote rejection, got %v", err)
	}
}
