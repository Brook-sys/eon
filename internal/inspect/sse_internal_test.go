package inspect

import (
	"testing"
	"time"
)

func TestSSEDrainPacerBoundsImmediatePagesAndResets(t *testing.T) {
	var pacer sseDrainPacer
	for page := 1; page < maxSSEImmediatePages; page++ {
		if !pacer.continueImmediately(true) {
			t.Fatalf("page %d yielded before burst limit %d", page, maxSSEImmediatePages)
		}
	}
	if pacer.continueImmediately(true) {
		t.Fatalf("page %d did not force timer yield", maxSSEImmediatePages)
	}
	if !pacer.continueImmediately(true) {
		t.Fatal("new burst did not resume immediate draining after forced yield")
	}
	if pacer.continueImmediately(false) {
		t.Fatal("finite backlog completion requested immediate continuation")
	}
	if !pacer.continueImmediately(true) {
		t.Fatal("backlog completion did not reset the next burst")
	}
}

func TestSSEKeepAliveCadenceIsDurationBoundedAcrossPollIntervals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		poll time.Duration
		want int
	}{
		{name: "minimum poll", poll: 50 * time.Millisecond, want: 200},
		{name: "default poll", poll: defaultSSEPollInterval, want: 40},
		{name: "uneven poll rounds up", poll: 3 * time.Second, want: 4},
		{name: "maximum poll", poll: 5 * time.Second, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sseIdleTicksBeforeKeepAlive(tt.poll)
			if got != tt.want {
				t.Fatalf("idle ticks before keepalive = %d, want %d", got, tt.want)
			}
			elapsed := time.Duration(got) * tt.poll
			if elapsed < sseKeepAliveInterval || elapsed >= sseKeepAliveInterval+tt.poll {
				t.Fatalf("keepalive elapsed = %s, want in [%s, %s)", elapsed, sseKeepAliveInterval, sseKeepAliveInterval+tt.poll)
			}
		})
	}
}
