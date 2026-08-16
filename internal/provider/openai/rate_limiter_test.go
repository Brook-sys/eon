// Package openai provides tests for the token bucket rate limiter.
package openai

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiterNilConfig(t *testing.T) {
	tb := newTokenBucket(nil)
	if tb != nil {
		t.Fatal("expected nil for nil config")
	}
	tb = newTokenBucket(&RateLimiterConfig{})
	if tb != nil {
		t.Fatal("expected nil for empty config")
	}
	tb = newTokenBucket(&RateLimiterConfig{RequestsPerMinute: 0, TokensPerMinute: 0})
	if tb != nil {
		t.Fatal("expected nil for zero RPM/TPM")
	}
}

func TestRateLimiterBasicAcquireRelease(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 60,
		TokensPerMinute:   6000,
		InitialBurst:      10,
	}
	tb := newTokenBucket(cfg)
	if tb == nil {
		t.Fatal("expected non-nil bucket")
	}

	ctx := context.Background()
	// Should acquire immediately from burst
	for i := 0; i < 10; i++ {
		if err := tb.Acquire(ctx, 1, 0); err != nil {
			t.Fatalf("acquire %d failed: %v", i, err)
		}
	}
	// 11th should wait (but we use timeout)
	err := tb.Acquire(ctx, 1, 10*time.Millisecond)
	if err != ErrRateLimitTimeout {
		t.Fatalf("expected timeout, got: %v", err)
	}
}

func TestRateLimiterTryTake(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 60,
		TokensPerMinute:   6000,
		InitialBurst:      5,
	}
	tb := newTokenBucket(cfg)

	// Should succeed for first 5
	for i := 0; i < 5; i++ {
		if !tb.tryTake(1) {
			t.Fatalf("tryTake %d should succeed", i)
		}
	}
	// 6th should fail
	if tb.tryTake(1) {
		t.Fatal("tryTake should fail when exhausted")
	}
}

func TestRateLimiterRefillOverTime(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 60, // 1 per second
		TokensPerMinute:   0,
		InitialBurst:      2,
	}
	tb := newTokenBucket(cfg)

	ctx := context.Background()
	// Use initial burst
	if err := tb.Acquire(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := tb.Acquire(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}
	// Third should fail with timeout
	err := tb.Acquire(ctx, 1, 50*time.Millisecond)
	if err != ErrRateLimitTimeout {
		t.Fatalf("expected timeout, got: %v", err)
	}
	// Wait for refill (~1 second for 1 token at 60 RPM)
	time.Sleep(1100 * time.Millisecond)
	// Should succeed now
	if err := tb.Acquire(ctx, 1, 100*time.Millisecond); err != nil {
		t.Fatalf("acquire after refill failed: %v", err)
	}
}

func TestRateLimiterReturnTokens(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 10,
		TokensPerMinute:   0,
		InitialBurst:      3,
	}
	tb := newTokenBucket(cfg)

	ctx := context.Background()
	// Acquire 3
	for i := 0; i < 3; i++ {
		if err := tb.Acquire(ctx, 1, 0); err != nil {
			t.Fatal(err)
		}
	}
	// Should be exhausted
	err := tb.Acquire(ctx, 1, 10*time.Millisecond)
	if err != ErrRateLimitTimeout {
		t.Fatalf("expected timeout, got: %v", err)
	}
	// Return one
	tb.Return(1)
	// Should succeed now
	if err := tb.Acquire(ctx, 1, 0); err != nil {
		t.Fatalf("acquire after return failed: %v", err)
	}
}

func TestRateLimiterConcurrent(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 1000, // High rate
		TokensPerMinute:   0,
		InitialBurst:      50,
	}
	tb := newTokenBucket(cfg)

	var wg sync.WaitGroup
	success := atomic.Int32{}
	failures := atomic.Int32{}

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := tb.Acquire(ctx, 1, 500*time.Millisecond); err != nil {
				failures.Add(1)
				return
			}
			success.Add(1)
		}()
	}
	wg.Wait()

	// With burst=50 and rate=1000/min, we should get most but not all
	// due to concurrent contention
	if success.Load() < 40 {
		t.Fatalf("too few successes: %d (failures: %d)", success.Load(), failures.Load())
	}
}

func TestRateLimiterCapacity(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 120,
		TokensPerMinute:   12000,
		InitialBurst:      1200, // > 100*10 so the test can take 10*100=1000 tokens
	}
	tb := newTokenBucket(cfg)

	reqCap, tokCap := tb.Capacity()
	if reqCap != 120 {
		t.Fatalf("request capacity: expected 120, got %d", reqCap)
	}
	if tokCap != 12000 {
		t.Fatalf("token capacity: expected 12000, got %d", tokCap)
	}

	// Use some (100 tokens × 10 = 1000 tokens; 10 request slots)
	for i := 0; i < 10; i++ {
		if !tb.tryTake(100) {
			t.Fatalf("tryTake %d should succeed", i)
		}
	}
	reqInUse, tokInUse := tb.InUse()
	if reqInUse != 10 {
		t.Fatalf("requests in use: expected 10, got %d", reqInUse)
	}
	if tokInUse != 1000 {
		t.Fatalf("tokens in use: expected 1000, got %d", tokInUse)
	}
}

func TestRateLimiterTokenLimit(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 1000,
		TokensPerMinute:   1000, // 1 token per second equivalent
		InitialBurst:      100,
	}
	tb := newTokenBucket(cfg)

	ctx := context.Background()
	// Request with large token count
	err := tb.Acquire(ctx, 200, 10*time.Millisecond)
	if err != ErrRateLimitTimeout {
		t.Fatalf("expected timeout for token limit, got: %v", err)
	}
	// Small token count should succeed
	if err := tb.Acquire(ctx, 50, 0); err != nil {
		t.Fatal(err)
	}
}

func TestRateLimiterWaitChannels(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 60,
		TokensPerMinute:   0,
		InitialBurst:      1,
	}
	tb := newTokenBucket(cfg)

	ctx := context.Background()
	// Take the one burst token
	if err := tb.Acquire(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}

	// Now start a goroutine waiting for a token
	done := make(chan error, 1)
	go func() {
		done <- tb.Acquire(ctx, 1, 2*time.Second)
	}()

	// Wait a bit, then the refill should trigger
	time.Sleep(1200 * time.Millisecond)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("waiter failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("waiter did not complete in time")
	}
}

func TestRateLimiterContextCancellation(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 10,
		TokensPerMinute:   0,
		InitialBurst:      1,
	}
	tb := newTokenBucket(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	// Exhaust burst
	if err := tb.Acquire(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}

	// Start waiting
	done := make(chan error, 1)
	go func() {
		done <- tb.Acquire(ctx, 1, 10*time.Second)
	}()

	// Cancel context
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("waiter did not respond to cancellation")
	}
}

func TestRateLimiterZeroCapacity(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 0,
		TokensPerMinute:   0,
	}
	tb := newTokenBucket(cfg)
	if tb != nil {
		t.Fatal("expected nil for zero capacity")
	}
}

func TestRateLimiterRequestsOnly(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 60,
		TokensPerMinute:   0,
		InitialBurst:      10,
	}
	tb := newTokenBucket(cfg)

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if err := tb.Acquire(ctx, 1, 0); err != nil {
			t.Fatal(err)
		}
	}
	err := tb.Acquire(ctx, 1, 10*time.Millisecond)
	if err != ErrRateLimitTimeout {
		t.Fatalf("expected timeout, got: %v", err)
	}
}

func TestRateLimiterTokensOnly(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerMinute: 0,
		TokensPerMinute:   6000,
		InitialBurst:      1000,
	}
	tb := newTokenBucket(cfg)

	ctx := context.Background()
	// Should succeed
	if err := tb.Acquire(ctx, 500, 0); err != nil {
		t.Fatal(err)
	}
	// Should fail - not enough tokens
	err := tb.Acquire(ctx, 600, 10*time.Millisecond)
	if err != ErrRateLimitTimeout {
		t.Fatalf("expected timeout, got: %v", err)
	}
}