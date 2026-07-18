package memory

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

func TestModelContextPressureSurvivesCheckpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 7, 10, 0, 0, time.UTC)
	store := New()
	want := domain.ModelContextPressure{
		BindingID: "nim-small",
		State:     domain.ContextPressureState{Level: 2, SuccessesAtLevel: 1},
		UpdatedAt: now,
	}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.SaveModelContextPressure(want) }); err != nil {
		t.Fatal(err)
	}
	payload, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewFromBinary(payload)
	if err != nil {
		t.Fatal(err)
	}
	var got domain.ModelContextPressure
	if err := restored.View(ctx, func(r port.Reader) error {
		got, err = r.ModelContextPressure(want.BindingID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("restored pressure = %+v, want %+v", got, want)
	}
}
