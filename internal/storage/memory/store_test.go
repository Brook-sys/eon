package memory_test

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/contract"
	"motor-autonomo/internal/storage/memory"
)

func TestStoreContract(t *testing.T) {
	contract.TestStore(t, func() port.Store { return memory.New() })
}

func TestUnsettledModelCompletionReceiptsAreOrderedBoundedAndExcludeLegacy(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 28, 17, 0, 0, 0, time.UTC)
	makeReceipt := func(op domain.OperationID, attempt, call uint32, permits bool) domain.ModelCompletionReceipt {
		result := domain.ModelCompletionResult{Text: string(op)}
		hash, _ := result.Hash()
		r := domain.ModelCompletionReceipt{SchemaVersion: 1, OperationID: op, Attempt: attempt, ModelCall: call, Result: result, PayloadHash: hash, RecordedAt: now}
		if permits {
			r.Permits = []domain.ResourcePermit{{Resource: domain.ResourceID("resource-" + string(op)), Cost: domain.ResourceCost{Slots: 1}, GrantedAt: now}}
		}
		return r
	}
	for _, r := range []domain.ModelCompletionReceipt{makeReceipt("z", 1, 1, true), makeReceipt("a", 2, 1, true), makeReceipt("a", 1, 2, true), makeReceipt("legacy", 1, 1, false)} {
		if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendModelCompletionReceipt(r) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.UnsettledModelCompletionReceipts(2)
		if err != nil {
			return err
		}
		if len(got) != 2 || got[0].OperationID != "a" || got[0].Attempt != 1 || got[1].OperationID != "a" || got[1].Attempt != 2 {
			t.Fatalf("ordered bounded receipts=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
