package inspect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestListModelContextPressuresEmptyAndPopulated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	store := memory.New()
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "test", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	empty, err := projector.ListModelContextPressures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Count != 0 || empty.Note == "" || empty.Pressures == nil {
		t.Fatalf("empty projection = %+v", empty)
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.SaveModelContextPressure(domain.ModelContextPressure{
			BindingID: "nim-small",
			State:     domain.ContextPressureState{Level: 2, SuccessesAtLevel: 1},
			UpdatedAt: now,
		}); err != nil {
			return err
		}
		return tx.SaveModelContextPressure(domain.ModelContextPressure{
			BindingID: "groq-primary",
			State:     domain.ContextPressureState{Level: 1},
			UpdatedAt: now.Add(time.Minute),
		})
	}); err != nil {
		t.Fatal(err)
	}

	proj, err := projector.ListModelContextPressures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if proj.Count != 2 || len(proj.Pressures) != 2 {
		t.Fatalf("proj = %+v", proj)
	}
	// Sorted by BindingID.
	if proj.Pressures[0].BindingID != "groq-primary" || proj.Pressures[1].BindingID != "nim-small" {
		t.Fatalf("order = %+v", proj.Pressures)
	}
	nim := proj.Pressures[1]
	if nim.Level != 2 || nim.SuccessesAtLevel != 1 || !nim.ReductionActive || nim.ReductionFraction != 0.5 {
		t.Fatalf("nim view = %+v", nim)
	}

	one, err := projector.ModelContextPressure(ctx, "nim-small")
	if err != nil {
		t.Fatal(err)
	}
	if one.Level != 2 {
		t.Fatalf("one = %+v", one)
	}
	if _, err := projector.ModelContextPressure(ctx, "missing"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestModelContextPressureHTTPEndpoints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 20, 10, 0, 0, time.UTC)
	store := memory.New()
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveModelContextPressure(domain.ModelContextPressure{
			BindingID: "nim-small",
			State:     domain.ContextPressureState{Level: 3},
			UpdatedAt: now,
		})
	}); err != nil {
		t.Fatal(err)
	}
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "test", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/model-context-pressures")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var list inspect.ModelContextPressuresProjection
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if list.Count != 1 || list.Pressures[0].BindingID != "nim-small" || list.Pressures[0].ReductionFraction != 0.25 {
		t.Fatalf("list = %+v", list)
	}

	resp2, err := http.Get(srv.URL + "/model-context-pressures/nim-small")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d", resp2.StatusCode)
	}
	var row inspect.ModelContextPressureView
	if err := json.NewDecoder(resp2.Body).Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.Level != 3 || !row.ReductionActive {
		t.Fatalf("row = %+v", row)
	}

	resp3, err := http.Get(srv.URL + "/model-context-pressures/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", resp3.StatusCode)
	}
}
