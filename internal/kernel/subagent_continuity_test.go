package kernel_test

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
)

type mockSubagentTaskSource struct {
	tasks []kernel.SubagentTask
	err   error
}

func (s *mockSubagentTaskSource) PendingSubagentTasks(ctx context.Context, revID domain.MissionRevisionID, limit int) ([]kernel.SubagentTask, error) {
	if s.err != nil {
		return nil, s.err
	}
	if limit > 0 && len(s.tasks) > limit {
		return s.tasks[:limit], nil
	}
	return s.tasks, nil
}

func TestSubagentContinuityFamilyName(t *testing.T) {
	family := kernel.SubagentContinuityFamily{}
	if family.Name() != "subagent_orchestration" {
		t.Errorf("expected name subagent_orchestration, got %s", family.Name())
	}
}

func TestSubagentContinuityFamilyReplenishSkipsIfUninitialized(t *testing.T) {
	family := kernel.SubagentContinuityFamily{}
	res, err := family.Replenish(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Admitted != 0 {
		t.Errorf("expected 0 admitted, got %d", res.Admitted)
	}
}

func TestSubagentContinuityFamilyReplenishDispatchesPendingTasks(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	sm := kernel.NewLocalSessionManager(clock)
	source := &mockSubagentTaskSource{
		tasks: []kernel.SubagentTask{
			{TaskID: "1", Task: "task1", ContextMode: "isolated"},
			{TaskID: "2", Task: "task2", ContextMode: "fork"},
		},
	}
	family := kernel.SubagentContinuityFamily{Manager: sm, Source: source}
	ctx := context.Background()

	res, err := family.Replenish(ctx, "test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Admitted != 2 || !res.Changed {
		t.Errorf("expected 2 admitted and Changed=true, got %v", res)
	}
}

func TestSubagentContinuityFamilyReplenishStopsOnConcurrencyLimit(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	sm, _ := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	source := &mockSubagentTaskSource{
		tasks: []kernel.SubagentTask{
			{TaskID: "1", Task: "task1", ContextMode: "isolated"},
			{TaskID: "2", Task: "task2", ContextMode: "isolated"},
		},
	}
	family := kernel.SubagentContinuityFamily{Manager: sm, Source: source}
	ctx := context.Background()

	res, err := family.Replenish(ctx, "test")
	if err != nil {
		t.Fatalf("expected no error on limit hit, got %v", err)
	}
	if res.Admitted != 1 || !res.Changed {
		t.Errorf("expected 1 admitted and Changed=true, got %v", res)
	}
}
