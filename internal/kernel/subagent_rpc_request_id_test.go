package kernel

import (
	"strings"
	"testing"
)

func TestDerivedSubagentRPCRequestIDIsBoundedStableAndFramed(t *testing.T) {
	maximum := strings.Repeat("x", 128)
	first := derivedSubagentRPCRequestID("subagent-status", "peer-a", maximum, "session-a", "3")
	if len(first) > 128 {
		t.Fatalf("request id is %d bytes: %q", len(first), first)
	}
	if repeat := derivedSubagentRPCRequestID("subagent-status", "peer-a", maximum, "session-a", "3"); repeat != first {
		t.Fatalf("derived request id is not stable: %q != %q", repeat, first)
	}
	if otherPeer := derivedSubagentRPCRequestID("subagent-status", "peer-b", maximum, "session-a", "3"); otherPeer == first {
		t.Fatal("peer identity did not affect derived request id")
	}
	if left := derivedSubagentRPCRequestID("subagent-status", "a", "bc"); left == derivedSubagentRPCRequestID("subagent-status", "ab", "c") {
		t.Fatal("field framing is ambiguous")
	}
}
