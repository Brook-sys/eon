// Package openai provides provider-side semaphore for coordinated concurrency control.
// This limits simultaneous outbound HTTP requests to stay within provider rate limits,
// complementing the kernel-level ResourceGate and preventing uncoordinated retry cascades.
package openai

import (
	"context"
	"errors"
	"time"
)

// SemaphoreConfig configures a provider-level concurrency semaphore.
type SemaphoreConfig struct {
	// MaxConcurrent is the maximum number of simultaneous requests allowed.
	// Zero disables the semaphore (unlimited concurrency).
	MaxConcurrent int
	// AcquireTimeout bounds how long a caller waits for a semaphore slot.
	// Zero means no timeout (wait indefinitely). Positive values cause
	// ErrSemaphoreTimeout after the duration expires.
	AcquireTimeout time.Duration
}

// semaphore is a lightweight counting semaphore using a channel.
type semaphore struct {
	ch chan struct{}
}

// newSemaphore creates a semaphore with the given capacity. Capacity <= 0
// returns a no-op semaphore that never blocks.
func newSemaphore(capacity int) *semaphore {
	if capacity <= 0 {
		return &semaphore{ch: nil}
	}
	return &semaphore{ch: make(chan struct{}, capacity)}
}

// Acquire blocks until a slot is available or ctx/timeout expires.
// Returns nil on success, or ErrSemaphoreTimeout if AcquireTimeout expired.
func (s *semaphore) Acquire(ctx context.Context, timeout time.Duration) error {
	if s.ch == nil {
		return nil
	}
	if timeout <= 0 {
		select {
		case s.ch <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-time.After(timeout):
		return ErrSemaphoreTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns a slot to the semaphore. Panics if called on a no-op semaphore.
func (s *semaphore) Release() {
	if s.ch == nil {
		return
	}
	select {
	case <-s.ch:
	default:
		// This should never happen if every Acquire is paired with Release.
		// Panic to surface programming errors.
		panic("semaphore: release without acquire")
	}
}

// SemaphoreSnapshot captures the current state of the semaphore for observability.
type SemaphoreSnapshot struct {
	Capacity     int
	InUse        int
	Available    int
	WaitQueueLen int // approximate, since Go channels don't expose queue length
}

// Snapshot returns the current state of the semaphore.
func (s *semaphore) Snapshot() SemaphoreSnapshot {
	if s.ch == nil {
		return SemaphoreSnapshot{Capacity: 0, InUse: 0, Available: 0, WaitQueueLen: 0}
	}
	capacity := cap(s.ch)
	inUse := len(s.ch)
	return SemaphoreSnapshot{
		Capacity:  capacity,
		InUse:     inUse,
		Available: capacity - inUse,
		WaitQueueLen: 0, // Go channels don't expose waiting goroutines count
	}
}

// ErrSemaphoreTimeout is returned when AcquireTimeout expires.
var ErrSemaphoreTimeout = errors.New("semaphore acquire timeout")

// WrapSemaphore wraps an operation with semaphore acquire/release.
func (p *Provider) WrapSemaphore(ctx context.Context, timeout time.Duration, op func() error) error {
	if p.sem == nil {
		return op()
	}
	if err := p.sem.Acquire(ctx, timeout); err != nil {
		return err
	}
	err := op()
	p.sem.Release()
	return err
}

