package kernel_test

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
)

func TestSubagentContinuityFamilyName(t *testing.T) {
	family := kernel.SubagentContinuityFamily{}
	if family.Name() != "subagent_orchestration" {
		t.Errorf("expected name subagent_orchestration, got %s", family.Name())
	}
}

func TestSubagentContinuityFamilyReplenish(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	sm := kernel.NewLocalSessionManager(clock)
	family := kernel.SubagentContinuityFamily{Manager: sm}
	res, err := family.Replenish(context.Background(), domain.MissionRevisionID("test"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Admitted != 0 {
		t.Errorf("expected 0 admitted, got %d", res.Admitted)
	}
}
