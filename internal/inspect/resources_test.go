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

func TestListResourceUsagesEmptyAndPopulated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	store := memory.New()
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "test", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	empty, err := projector.ListResourceUsages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Count != 0 || empty.Note == "" {
		t.Fatalf("empty projection = %+v", empty)
	}

	openUntil := now.Add(2 * time.Minute)
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveResourceUsage(domain.ResourceUsage{
			Resource:            "model:default",
			InFlight:            1,
			MinuteCount:         3,
			DayCount:            10,
			TokenMinuteCount:    500,
			ConsecutiveFailures: 2,
			MinuteWindowStart:   now.Truncate(time.Minute),
			DayWindowStart:      now.Truncate(24 * time.Hour),
			CircuitOpenUntil:    &openUntil,
		})
	}); err != nil {
		t.Fatal(err)
	}

	proj, err := projector.ListResourceUsages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if proj.Count != 1 || len(proj.Resources) != 1 {
		t.Fatalf("proj = %+v", proj)
	}
	row := proj.Resources[0]
	if row.Resource != "model:default" || row.InFlight != 1 || !row.CircuitOpen {
		t.Fatalf("row = %+v", row)
	}

	one, err := projector.ResourceUsage(ctx, "model:default")
	if err != nil {
		t.Fatal(err)
	}
	if one.DayCount != 10 {
		t.Fatalf("one = %+v", one)
	}
	if _, err := projector.ResourceUsage(ctx, "missing"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestResourcesHTTPEndpoints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 22, 10, 0, 0, time.UTC)
	store := memory.New()
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveResourceUsage(domain.ResourceUsage{
			Resource: "web:http", InFlight: 0, MinuteCount: 1,
			MinuteWindowStart: now.Truncate(time.Minute),
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

	resp, err := http.Get(srv.URL + "/resources")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var list inspect.ResourcesProjection
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if list.Count != 1 || list.Resources[0].Resource != "web:http" {
		t.Fatalf("list = %+v", list)
	}

	resp2, err := http.Get(srv.URL + "/resources/web:http")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d", resp2.StatusCode)
	}
	var row inspect.ResourceUsageView
	if err := json.NewDecoder(resp2.Body).Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.MinuteCount != 1 {
		t.Fatalf("row = %+v", row)
	}

	resp3, err := http.Get(srv.URL + "/resources/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", resp3.StatusCode)
	}
}
