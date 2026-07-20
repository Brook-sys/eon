package domain

import (
	"testing"
	"time"
)

func TestSubagentReconcileRPCStrictAndDigestSensitive(t *testing.T) {
	spawn := SubagentSpawnRequest{RequestID: "req-1", SessionID: "session-1", Attempt: 2, Task: "work", ContextMode: "isolated"}
	digest, err := SubagentSpawnRequestDigest(spawn)
	if err != nil || len(digest) != 64 {
		t.Fatalf("digest=(%q,%v)", digest, err)
	}
	spawn.Task = "different"
	other, _ := SubagentSpawnRequestDigest(spawn)
	if digest == other {
		t.Fatal("digest did not bind complete spawn payload")
	}
	request := SubagentReconcileRequest{Kind: SubagentReconcileSpawn, DeliveryID: "req-1", SessionID: "session-1", Attempt: 2, Digest: digest}
	payload, err := EncodeSubagentReconcileRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSubagentReconcileRequest(payload)
	if err != nil || decoded != request {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
	if _, err := DecodeSubagentReconcileRequest(append(payload, []byte(` {}`)...)); err == nil {
		t.Fatal("accepted trailing JSON")
	}
}

func TestResolveSubagentStatusDeliveryAfterPositiveReconcile(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	for _, state := range []SubagentStatusDeliveryState{SubagentStatusDeliveryInFlight, SubagentStatusDeliveryEffectUnknown} {
		receipt := SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: "peer-a", RequestID: "req-1", SourceSessionID: "source-1", Task: "work", ContextMode: "isolated", ReceiverSessionID: "receiver-1", RecordedAt: now, Status: SubagentSpawnReceiptComplete, UpdatedAt: now, Result: "answer", StatusDelivery: state}
		next, err := ResolveSubagentSpawnReceiptStatusFound(receipt, now.Add(time.Second))
		if err != nil || next.StatusDelivery != SubagentStatusDeliveryDelivered || next.Result != receipt.Result {
			t.Fatalf("state=%s next=%+v err=%v", state, next, err)
		}
	}
}
