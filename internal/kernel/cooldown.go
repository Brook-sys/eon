package kernel

import (
	"strings"
	"time"
)

// StrategyCooldown tracks last unsuccessful attempt to avoid fixation.
type StrategyCooldown struct {
	Name          string
	CooldownUntil time.Time
}

func (c StrategyCooldown) Active(now time.Time) bool {
	return !c.CooldownUntil.IsZero() && c.CooldownUntil.After(now.UTC())
}

// StrategyCooldownBook is process-local memory of recent no-delta strategy
// attempts. It is not model-authored and does not replace durable diagnosis.
type StrategyCooldownBook struct {
	entries map[string]time.Time
}

// NewStrategyCooldownBook returns an empty book.
func NewStrategyCooldownBook() *StrategyCooldownBook {
	return &StrategyCooldownBook{entries: make(map[string]time.Time)}
}

func (b *StrategyCooldownBook) ensure() {
	if b.entries == nil {
		b.entries = make(map[string]time.Time)
	}
}

// Active reports whether the named strategy is still cooling down.
func (b *StrategyCooldownBook) Active(name string, now time.Time) bool {
	if b == nil {
		return false
	}
	b.ensure()
	until, ok := b.entries[name]
	return ok && until.After(now.UTC())
}

// MarkNoDelta records that a strategy ran without admitting executable work.
func (b *StrategyCooldownBook) MarkNoDelta(name string, now time.Time, cooldown time.Duration) {
	if b == nil || strings.TrimSpace(name) == "" || cooldown <= 0 {
		return
	}
	b.ensure()
	b.entries[name] = now.UTC().Add(cooldown)
}

// Clear removes a cooldown after a successful admission or explicit recovery.
func (b *StrategyCooldownBook) Clear(name string) {
	if b == nil || b.entries == nil {
		return
	}
	delete(b.entries, name)
}
