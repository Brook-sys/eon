// Package source provides injectable sources of time, identity and randomness.
// Official kernel decisions must depend on these ports rather than package-level
// clocks or random generators (FR-DUR-005, INV-DUR-006).
package source

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
	WaitUntil(context.Context, time.Time) error
}

type IDGenerator interface {
	NewID(prefix string) (string, error)
}

type RandomSource interface {
	Uint64() (uint64, error)
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

func (SystemClock) WaitUntil(ctx context.Context, deadline time.Time) error {
	delay := time.Until(deadline)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type CryptoIDGenerator struct{}

func (CryptoIDGenerator) NewID(prefix string) (string, error) {
	if prefix == "" {
		return "", errors.New("ID prefix must not be empty")
	}
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate ID entropy: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(value[:]), nil
}

type CryptoRandomSource struct{}

func (CryptoRandomSource) Uint64() (uint64, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return 0, fmt.Errorf("generate random value: %w", err)
	}
	return binary.BigEndian.Uint64(value[:]), nil
}

// ManualClock is safe for tests that exercise concurrent scheduling code.
type ManualClock struct {
	mu      sync.RWMutex
	now     time.Time
	changed chan struct{}
}

func NewManualClock(now time.Time) *ManualClock {
	return &ManualClock{now: now.UTC(), changed: make(chan struct{})}
}

func (c *ManualClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *ManualClock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now.UTC()
	c.signalLocked()
}

func (c *ManualClock) Advance(delta time.Duration) error {
	if delta < 0 {
		return errors.New("manual clock cannot move backwards")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delta)
	c.signalLocked()
	return nil
}

func (c *ManualClock) WaitUntil(ctx context.Context, deadline time.Time) error {
	for {
		c.mu.RLock()
		now, changed := c.now, c.changed
		c.mu.RUnlock()
		if !now.Before(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (c *ManualClock) signalLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// SequenceIDGenerator yields reproducible, process-local IDs for tests. The
// counter is monotonic and protected so race tests can reuse it safely.
type SequenceIDGenerator struct {
	mu   sync.Mutex
	next uint64
}

func NewSequenceIDGenerator(first uint64) *SequenceIDGenerator {
	return &SequenceIDGenerator{next: first}
}

func (g *SequenceIDGenerator) NewID(prefix string) (string, error) {
	if prefix == "" {
		return "", errors.New("ID prefix must not be empty")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	value := g.next
	g.next++
	return fmt.Sprintf("%s_%016x", prefix, value), nil
}

// SequenceRandomSource returns a scripted sequence and fails explicitly when
// exhausted; it never silently falls back to nondeterministic randomness.
type SequenceRandomSource struct {
	mu     sync.Mutex
	values []uint64
	next   int
}

func NewSequenceRandomSource(values ...uint64) *SequenceRandomSource {
	copyOfValues := append([]uint64(nil), values...)
	return &SequenceRandomSource{values: copyOfValues}
}

func (s *SequenceRandomSource) Uint64() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next >= len(s.values) {
		return 0, errors.New("random sequence exhausted")
	}
	value := s.values[s.next]
	s.next++
	return value, nil
}

// ConstantRandomSource returns a fixed uint64 value indefinitely.
type ConstantRandomSource struct {
	value uint64
}

func NewConstantRandomSource(value uint64) ConstantRandomSource {
	return ConstantRandomSource{value: value}
}

func (c ConstantRandomSource) Uint64() (uint64, error) {
	return c.value, nil
}
