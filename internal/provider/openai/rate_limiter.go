// Package openai provides a token bucket rate limiter for provider-level
// rate limit enforcement. This complements the concurrency semaphore by
// gating requests based on rate over time (RPM/TPM) rather than simultaneous
// in-flight count. It uses a sliding window with refill based on wall clock.
package openai

import (
	"context"
	"errors"
	"sync"
	"time"
)

// RateLimiterConfig configures a token bucket rate limiter.
type RateLimiterConfig struct {
	// RequestsPerMinute is the maximum requests allowed per minute.
	// Zero disables request rate limiting.
	RequestsPerMinute int
	// TokensPerMinute is the maximum tokens allowed per minute.
	// Zero disables token rate limiting.
	TokensPerMinute int
	// InitialBurst is the initial bucket capacity (tokens available at start).
	// If zero, defaults to RequestsPerMinute/6 (10 seconds of headroom).
	InitialBurst int
	// AcquireTimeout bounds how long a caller waits for tokens.
	// Zero means no timeout (wait indefinitely).
	AcquireTimeout time.Duration
}

// ErrRateLimitTimeout is returned when AcquireTimeout expires.
var ErrRateLimitTimeout = errors.New("rate limiter acquire timeout")

// tokenBucket implements a sliding-window token bucket for rate limiting.
type tokenBucket struct {
	mu sync.Mutex

	// Request bucket
	reqCapacity   int
	reqTokens     float64
	reqRefillRate float64 // tokens per nanosecond
	reqLastRefill time.Time
	reqConsumed    int // total requests taken (for observability)

	// Token bucket
	tokCapacity   int
	tokTokens     float64
	tokRefillRate float64 // tokens per nanosecond
	tokLastRefill time.Time
	tokConsumed    int // total tokens taken (for observability)

	// For waiting
	waitCh chan struct{}
}

// newTokenBucket creates a token bucket from config.
// If both RPM and TPM are zero, returns a no-op limiter (nil).
func newTokenBucket(cfg *RateLimiterConfig) *tokenBucket {
	if cfg == nil || (cfg.RequestsPerMinute <= 0 && cfg.TokensPerMinute <= 0) {
		return nil
	}
	now := time.Now()
	tb := &tokenBucket{
		reqCapacity:   cfg.RequestsPerMinute,
		reqTokens:     float64(cfg.RequestsPerMinute),
		reqRefillRate: float64(cfg.RequestsPerMinute) / float64(time.Minute),
		reqLastRefill: now,
		tokCapacity:   cfg.TokensPerMinute,
		tokTokens:     float64(cfg.TokensPerMinute),
		tokRefillRate: float64(cfg.TokensPerMinute) / float64(time.Minute),
		tokLastRefill: now,
		waitCh:        make(chan struct{}, 1),
	}
	// Apply initial burst if configured
	if cfg.InitialBurst > 0 {
		tb.reqTokens = float64(cfg.InitialBurst)
		if cfg.RequestsPerMinute > 0 && tb.reqTokens > float64(cfg.RequestsPerMinute) {
			tb.reqTokens = float64(cfg.RequestsPerMinute)
		}
		if cfg.TokensPerMinute > 0 {
			tb.tokTokens = float64(cfg.InitialBurst)
			if tb.tokTokens > float64(cfg.TokensPerMinute) {
				tb.tokTokens = float64(cfg.TokensPerMinute)
			}
		}
	}
	return tb
}

// Acquire blocks until the requested number of request tokens (always 1)
// and token tokens (estimated or actual) are available, or ctx/timeout expires.
// Returns nil on success, or ErrRateLimitTimeout if AcquireTimeout expired.
func (tb *tokenBucket) Acquire(ctx context.Context, tokens int, timeout time.Duration) error {
	if tb == nil {
		return nil
	}
	if tokens <= 0 {
		tokens = 1
	}

	if timeout <= 0 {
		// Wait indefinitely
		for {
			if tb.tryTake(tokens) {
				return nil
			}
			// Wait for refill signal or context cancellation
			select {
			case <-tb.waitCh:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// Wait with timeout
	deadline := time.Now().Add(timeout)
	for {
		if tb.tryTake(tokens) {
			return nil
		}
		// Calculate wait time until next refill
		wait := tb.timeUntilTokens(tokens)
		if wait <= 0 {
			// Should be available now, retry immediately
			continue
		}
		if time.Now().Add(wait).After(deadline) {
			return ErrRateLimitTimeout
		}
		select {
		case <-tb.waitCh:
			continue
		case <-time.After(wait):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// tryTake attempts to consume tokens without blocking.
// Returns true if successful, false if not enough tokens.
func (tb *tokenBucket) tryTake(tokens int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked()

	if tb.reqCapacity > 0 && tb.reqTokens < 1 {
		return false
	}
	if tb.tokCapacity > 0 && tb.tokTokens < float64(tokens) {
		return false
	}

	// Consume tokens
	if tb.reqCapacity > 0 {
		tb.reqTokens -= 1
		tb.reqConsumed++
	}
	if tb.tokCapacity > 0 {
		tb.tokTokens -= float64(tokens)
		tb.tokConsumed += tokens
	}
	return true
}

// refillLocked refills both buckets based on elapsed time.
// Must hold tb.mu.
func (tb *tokenBucket) refillLocked() {
	now := time.Now()

	if tb.reqCapacity > 0 {
		elapsed := now.Sub(tb.reqLastRefill)
		refill := float64(elapsed) * tb.reqRefillRate
		tb.reqTokens += refill
		if tb.reqTokens > float64(tb.reqCapacity) {
			tb.reqTokens = float64(tb.reqCapacity)
		}
		tb.reqLastRefill = now
	}

	if tb.tokCapacity > 0 {
		elapsed := now.Sub(tb.tokLastRefill)
		refill := float64(elapsed) * tb.tokRefillRate
		tb.tokTokens += refill
		if tb.tokTokens > float64(tb.tokCapacity) {
			tb.tokTokens = float64(tb.tokCapacity)
		}
		tb.tokLastRefill = now
	}

	// Signal waiters if we have tokens
	if (tb.reqCapacity == 0 || tb.reqTokens >= 1) &&
		(tb.tokCapacity == 0 || tb.tokTokens >= 1) {
		select {
		case tb.waitCh <- struct{}{}:
		default:
		}
	}
}

// timeUntilTokens estimates the time until the requested tokens are available.
// Returns 0 if tokens should be available now (caller should retry).
func (tb *tokenBucket) timeUntilTokens(tokens int) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked()

	var maxWait time.Duration
	if tb.reqCapacity > 0 && tb.reqTokens < 1 {
		need := 1 - tb.reqTokens
		wait := time.Duration(float64(time.Minute) * need / float64(tb.reqCapacity))
		if wait > maxWait {
			maxWait = wait
		}
	}
	if tb.tokCapacity > 0 && tb.tokTokens < float64(tokens) {
		need := float64(tokens) - tb.tokTokens
		wait := time.Duration(float64(time.Minute) * need / float64(tb.tokCapacity))
		if wait > maxWait {
			maxWait = wait
		}
	}
	return maxWait
}

// Return returns tokens to the bucket (for rollback on error before dispatch).
// This is best-effort and does not exceed capacity.
func (tb *tokenBucket) Return(tokens int) {
	if tb == nil || tokens <= 0 {
		return
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.reqCapacity > 0 {
		tb.reqTokens += 1
		if tb.reqTokens > float64(tb.reqCapacity) {
			tb.reqTokens = float64(tb.reqCapacity)
		}
		if tb.reqConsumed > 0 {
			tb.reqConsumed--
		}
	}
	if tb.tokCapacity > 0 {
		tb.tokTokens += float64(tokens)
		if tb.tokTokens > float64(tb.tokCapacity) {
			tb.tokTokens = float64(tb.tokCapacity)
		}
		if tb.tokConsumed >= tokens {
			tb.tokConsumed -= tokens
		}
	}
	// Signal waiters
	select {
	case tb.waitCh <- struct{}{}:
	default:
	}
}

// InUse returns the number of tokens consumed since creation (for observability).
func (tb *tokenBucket) InUse() (requests int, tokens int) {
	if tb == nil {
		return 0, 0
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.reqConsumed, tb.tokConsumed
}

// Capacity returns the configured limits.
func (tb *tokenBucket) Capacity() (requests int, tokens int) {
	if tb == nil {
		return 0, 0
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.reqCapacity, tb.tokCapacity
}