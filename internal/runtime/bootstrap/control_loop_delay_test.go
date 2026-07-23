package bootstrap

import (
	"testing"
	"time"
)

func TestNextControlCycleDelayPrioritizesBoundedIngressRecovery(t *testing.T) {
	result := CycleResult{Worked: true, SubagentIngressRecoveryDelay: 125 * time.Millisecond}
	delay, nextIdle := nextControlCycleDelay(result, 800*time.Millisecond, 50*time.Millisecond, time.Second)
	if delay != 125*time.Millisecond || nextIdle != 50*time.Millisecond {
		t.Fatalf("delay=%s next_idle=%s", delay, nextIdle)
	}
}

func TestNextControlCycleDelayKeepsProductiveAndIdleCadence(t *testing.T) {
	if delay, next := nextControlCycleDelay(CycleResult{Worked: true}, 200*time.Millisecond, 50*time.Millisecond, time.Second); delay != 0 || next != 50*time.Millisecond {
		t.Fatalf("worked delay=%s next=%s", delay, next)
	}
	if delay, next := nextControlCycleDelay(CycleResult{}, 800*time.Millisecond, 50*time.Millisecond, time.Second); delay != 800*time.Millisecond || next != time.Second {
		t.Fatalf("idle delay=%s next=%s", delay, next)
	}
}
