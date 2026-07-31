package source

import (
	"testing"
	"time"
)

func TestManualClock(t *testing.T) {
	start := time.Date(2026, 7, 15, 12, 0, 0, 0, time.FixedZone("local", -3*60*60))
	clock := NewManualClock(start)
	if got := clock.Now(); !got.Equal(start) || got.Location() != time.UTC {
		t.Fatalf("Now() = %v, want UTC instant %v", got, start.UTC())
	}
	if err := clock.Advance(5 * time.Minute); err != nil {
		t.Fatalf("Advance() error = %v", err)
	}
	if got := clock.Now(); !got.Equal(start.Add(5 * time.Minute)) {
		t.Fatalf("Now() after advance = %v", got)
	}
	if err := clock.Advance(-time.Second); err == nil {
		t.Fatal("Advance() allowed time to move backwards")
	}
}

func TestSequenceIDGenerator(t *testing.T) {
	generator := NewSequenceIDGenerator(10)
	first, err := generator.NewID("operation")
	if err != nil {
		t.Fatal(err)
	}
	second, err := generator.NewID("operation")
	if err != nil {
		t.Fatal(err)
	}
	if first != "operation_000000000000000a" || second != "operation_000000000000000b" {
		t.Fatalf("unexpected sequence: %q, %q", first, second)
	}
}

func TestSequenceRandomSourceFailsWhenExhausted(t *testing.T) {
	source := NewSequenceRandomSource(7, 11)
	for _, want := range []uint64{7, 11} {
		got, err := source.Uint64()
		if err != nil || got != want {
			t.Fatalf("Uint64() = %d, %v; want %d, nil", got, err, want)
		}
	}
	if _, err := source.Uint64(); err == nil {
		t.Fatal("Uint64() did not report sequence exhaustion")
	}
}

func TestConstantRandomSource(t *testing.T) {
	src := NewConstantRandomSource(42)
	for i := 0; i < 10; i++ {
		got, err := src.Uint64()
		if err != nil || got != 42 {
			t.Fatalf("Uint64() = %d, %v; want 42, nil", got, err)
		}
	}
}
