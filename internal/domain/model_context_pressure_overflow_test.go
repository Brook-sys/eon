package domain_test

import (
	"testing"
	"motor-autonomo/internal/domain"
)

func TestContextPressureLevelOverflowBounded(t *testing.T) {
	state := domain.ContextPressureState{}
	
	// Increment past MaxContextPressureLevel
	for i := 0; i < domain.MaxContextPressureLevel+5; i++ {
		state = domain.RecordContextPressure(state)
	}
	
	if state.Level != domain.MaxContextPressureLevel {
		t.Fatalf("expected level to be capped at %d, but got %d", domain.MaxContextPressureLevel, state.Level)
	}
	
	if state.SuccessesAtLevel != 0 {
		t.Fatalf("expected successes to be reset to 0, got %d", state.SuccessesAtLevel)
	}
}
