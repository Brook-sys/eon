package peersync_test

import (
  "context"
  "testing"
  "time"

  "motor-autonomo/internal/domain"
  peersync "motor-autonomo/internal/network/sync"
  "motor-autonomo/internal/port"
  "motor-autonomo/internal/storage/memory"
)

type testPeerCallerFunc func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)

func (f testPeerCallerFunc) Call(ctx context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
  return f(ctx, request)
}

func TestBidirectionalSyncWithResolution(t *testing.T) {
  ctx := context.Background()
  now := time.Unix(100, 0).UTC()
  storeA := memory.New()
  storeB := memory.New()

  storeA.Update(ctx, func(tx port.Transaction) error {
    tx.AppendEvent(domain.Event{SchemaVersion: domain.SchemaVersionV1, ID: "event-1", Kind: "test", Sequence: 0, OccurredAt: now})
    return nil
  })

  storeB.Update(ctx, func(tx port.Transaction) error {
    tx.AppendEvent(domain.Event{SchemaVersion: domain.SchemaVersionV1, ID: "event-2", Kind: "test", Sequence: 0, OccurredAt: now})
    return nil
  })

  serviceA, _ := peersync.NewService(storeA, func() time.Time { return now })
  serviceB, _ := peersync.NewService(storeB, func() time.Time { return now })
  canonA, _ := peersync.NewBoundedInboxCanonicalizer(storeA, peersync.NewBasicConflictResolver())
  canonB, _ := peersync.NewBoundedInboxCanonicalizer(storeB, peersync.NewBasicConflictResolver())

  callerB := testPeerCallerFunc(func(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
    msg, _ := peersync.Decode(req.Payload)
    res, _ := serviceB.Handle(ctx, "node-a", "node-b", msg)
    payload, _ := peersync.Encode(res)
    return domain.PeerRPCResponse{RequestID: req.RequestID, PeerID: req.PeerID, Payload: payload}, nil
  })

  callerA := testPeerCallerFunc(func(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
    msg, _ := peersync.Decode(req.Payload)
    res, _ := serviceA.Handle(ctx, "node-b", "node-a", msg)
    payload, _ := peersync.Encode(res)
    return domain.PeerRPCResponse{RequestID: req.RequestID, PeerID: req.PeerID, Payload: payload}, nil
  })

  _, err := serviceA.PullOnce(ctx, callerB, "node-b", "node-a", "main", nil)
  if err != nil { t.Fatalf("A pull B: %v", err) }

  _, err = serviceB.PullOnce(ctx, callerA, "node-a", "node-b", "main", nil)
  if err != nil { t.Fatalf("B pull A: %v", err) }

  recA, err := canonA.Reconcile(ctx, "node-b")
  if err != nil { t.Fatalf("A reconcile: %v", err) }
  if recA != 1 { t.Fatalf("A expected 1 reconciled, got %d", recA) }

  recB, err := canonB.Reconcile(ctx, "node-a")
  if err != nil { t.Fatalf("B reconcile: %v", err) }
  if recB != 1 { t.Fatalf("B expected 1 reconciled, got %d", recB) }

  verifyLog := func(store port.Store, name string) {
    store.View(ctx, func(r port.Reader) error {
      events, _ := r.Events(0, 100)
      if len(events) != 2 {
        t.Fatalf("%s expected 2 events, got %d", name, len(events))
      }
      has1, has2 := false, false
      for _, e := range events {
        if e.ID == "event-1" { has1 = true }
        if e.ID == "event-2" { has2 = true }
      }
      if !has1 || !has2 {
        t.Fatalf("%s missing events: has1=%v has2=%v", name, has1, has2)
      }
      return nil
    })
  }

  verifyLog(storeA, "Node A")
  verifyLog(storeB, "Node B")
}
