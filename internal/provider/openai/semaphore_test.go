package openai

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSemaphoreBasicAcquireRelease(t *testing.T) {
	s := newSemaphore(3)
	
	if err := s.Acquire(context.Background(), 0); err != nil {
		t.Fatalf("acquire 1 failed: %v", err)
	}
	if err := s.Acquire(context.Background(), 0); err != nil {
		t.Fatalf("acquire 2 failed: %v", err)
	}
	if err := s.Acquire(context.Background(), 0); err != nil {
		t.Fatalf("acquire 3 failed: %v", err)
	}
	
	// Should block now
	done := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		s.Release()
		close(done)
	}()
	
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("acquire 4 should have succeeded after release")
	}
	
	if err := s.Acquire(context.Background(), 0); err != nil {
		t.Fatalf("acquire 4 failed after release: %v", err)
	}
	s.Release()
}

func TestSemaphoreSnapshotReflectsState(t *testing.T) {
	s := newSemaphore(5)
	
	// Initial state: all available
	snap := s.Snapshot()
	if snap.Capacity != 5 {
		t.Fatalf("expected capacity 5, got %d", snap.Capacity)
	}
	if snap.InUse != 0 {
		t.Fatalf("expected 0 in use, got %d", snap.InUse)
	}
	if snap.Available != 5 {
		t.Fatalf("expected 5 available, got %d", snap.Available)
	}
	
	// Acquire 3, check state
	for i := 0; i < 3; i++ {
		if err := s.Acquire(context.Background(), 0); err != nil {
			t.Fatalf("acquire %d failed: %v", i, err)
		}
	}
	snap = s.Snapshot()
	if snap.Capacity != 5 {
		t.Fatalf("expected capacity 5, got %d", snap.Capacity)
	}
	if snap.InUse != 3 {
		t.Fatalf("expected 3 in use, got %d", snap.InUse)
	}
	if snap.Available != 2 {
		t.Fatalf("expected 2 available, got %d", snap.Available)
	}
	
	// Release 2, check state
	s.Release()
	s.Release()
	snap = s.Snapshot()
	if snap.InUse != 1 {
		t.Fatalf("expected 1 in use after releases, got %d", snap.InUse)
	}
	if snap.Available != 4 {
		t.Fatalf("expected 4 available after releases, got %d", snap.Available)
	}
}

func TestSemaphoreSnapshotNilChannel(t *testing.T) {
	s := newSemaphore(0)
	snap := s.Snapshot()
	if snap.Capacity != 0 {
		t.Fatalf("expected capacity 0, got %d", snap.Capacity)
	}
	if snap.InUse != 0 {
		t.Fatalf("expected 0 in use, got %d", snap.InUse)
	}
	if snap.Available != 0 {
		t.Fatalf("expected 0 available, got %d", snap.Available)
	}
}

func TestSemaphoreZeroCapacityNoOp(t *testing.T) {
	s := newSemaphore(0)
	if err := s.Acquire(context.Background(), 0); err != nil {
		t.Fatalf("acquire on no-op semaphore failed: %v", err)
	}
	s.Release() // should not panic
}

func TestSemaphoreNegativeCapacityNoOp(t *testing.T) {
	s := newSemaphore(-1)
	if err := s.Acquire(context.Background(), 0); err != nil {
		t.Fatalf("acquire on negative capacity semaphore failed: %v", err)
	}
	s.Release() // should not panic
}

func TestSemaphoreConcurrentAccess(t *testing.T) {
	s := newSemaphore(5)
	const workers = 20
	const acquiresPerWorker = 5
	
	var wg sync.WaitGroup
	errors := make(chan error, workers*acquiresPerWorker)
	
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < acquiresPerWorker; i++ {
				if err := s.Acquire(context.Background(), 0); err != nil {
					errors <- err
					return
				}
				time.Sleep(time.Microsecond)
				s.Release()
			}
		}()
	}
	
	wg.Wait()
	close(errors)
	
	for err := range errors {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestSemaphoreTimeout(t *testing.T) {
	s := newSemaphore(1)
	
	// Take the only slot
	if err := s.Acquire(context.Background(), 0); err != nil {
		t.Fatalf("initial acquire failed: %v", err)
	}
	
	// Try to acquire with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	
	err := s.Acquire(ctx, 20*time.Millisecond)
	if err != ErrSemaphoreTimeout {
		t.Errorf("expected ErrSemaphoreTimeout, got %v", err)
	}
	
	// Release and try again - should succeed now
	s.Release()
	if err := s.Acquire(context.Background(), 20*time.Millisecond); err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
}

func TestSemaphoreContextCancellation(t *testing.T) {
	s := newSemaphore(1)
	
	// Take the slot
	if err := s.Acquire(context.Background(), 0); err != nil {
		t.Fatalf("initial acquire failed: %v", err)
	}
	
	// Try to acquire with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	
	err := s.Acquire(ctx, 0)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestSemaphorePanicOnReleaseWithoutAcquire(t *testing.T) {
	s := newSemaphore(1)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on release without acquire")
		}
	}()
	s.Release()
}

func TestProviderSemaphoreIntegration(t *testing.T) {
	// Test that provider correctly initializes semaphore
	cfg := Config{
		APIKey:  "test",
		BaseURL: "http://localhost",
		Model:   "test",
		Semaphore: &SemaphoreConfig{
			MaxConcurrent:   3,
			AcquireTimeout:  100 * time.Millisecond,
		},
	}
	
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	
	if p.sem == nil {
		t.Error("semaphore should be initialized")
	}
	if p.sem.ch == nil {
		t.Error("semaphore channel should be created for positive capacity")
	}
	if p.semAcquireTimeout != 100*time.Millisecond {
		t.Errorf("semAcquireTimeout mismatch: got %v, want 100ms", p.semAcquireTimeout)
	}
}

func TestProviderSemaphoreDisabled(t *testing.T) {
	cfg := Config{
		APIKey:  "test",
		BaseURL: "http://localhost",
		Model:   "test",
		Semaphore: &SemaphoreConfig{
			MaxConcurrent: 0,
		},
	}
	
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	
	if p.sem == nil || p.sem.ch != nil {
		t.Error("semaphore should be no-op (nil channel) for MaxConcurrent=0")
	}
}

func TestProviderSemaphoreNilConfig(t *testing.T) {
	cfg := Config{
		APIKey:  "test",
		BaseURL: "http://localhost",
		Model:   "test",
		Semaphore: nil,
	}
	
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	
	if p.sem == nil || p.sem.ch != nil {
		t.Error("semaphore should be no-op (nil channel) for nil Semaphore config")
	}
}

func TestWrapSemaphore(t *testing.T) {
	cfg := Config{
		APIKey:  "test",
		BaseURL: "http://localhost",
		Model:   "test",
		Semaphore: &SemaphoreConfig{
			MaxConcurrent:   2,
			AcquireTimeout:  50 * time.Millisecond,
		},
	}
	
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	
	// Test successful wrap
	callCount := 0
	err = p.WrapSemaphore(context.Background(), 50*time.Millisecond, func() error {
		callCount++
		return nil
	})
	if err != nil {
		t.Errorf("WrapSemaphore failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
	
	// Test that semaphore limits concurrency
	var wg sync.WaitGroup
	results := make(chan error, 5)
	
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := p.WrapSemaphore(context.Background(), 50*time.Millisecond, func() error {
				time.Sleep(20 * time.Millisecond)
				return nil
			})
			results <- err
		}()
	}
	
	wg.Wait()
	close(results)
	
	successCount := 0
	timeoutCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else if err == ErrSemaphoreTimeout {
			timeoutCount++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}
	
	// With capacity 2 and 5 workers each holding for 20ms with 50ms timeout,
	// first 2 succeed immediately, next 2 wait and succeed, last 1 might timeout
	// or succeed depending on timing. At minimum 2 should succeed.
	if successCount < 2 {
		t.Errorf("expected at least 2 successes, got %d", successCount)
	}
}

