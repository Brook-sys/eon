package domain

import (
	"errors"
	"fmt"
	"time"
)

// Budget arithmetic helpers for FR-RES-001 / INV-RES budget monotonicity.
// Zero on any dimension means "no units authorized" for that dimension, never unlimited.

// IsZero reports whether every dimension authorizes nothing.
func (b Budget) IsZero() bool {
	return b.ModelCalls == 0 && b.Tokens == 0 && b.Bytes == 0 && b.Attempts == 0 && b.Duration == 0
}

// Covers reports whether b authorizes at least cost on every dimension that
// cost requests. A zero cost always covers. A zero allowance never covers a
// positive cost on that dimension.
func (b Budget) Covers(cost Budget) bool {
	if cost.ModelCalls > 0 && b.ModelCalls < cost.ModelCalls {
		return false
	}
	if cost.Tokens > 0 && b.Tokens < cost.Tokens {
		return false
	}
	if cost.Bytes > 0 && b.Bytes < cost.Bytes {
		return false
	}
	if cost.Attempts > 0 && b.Attempts < cost.Attempts {
		return false
	}
	if cost.Duration > 0 && b.Duration < cost.Duration {
		return false
	}
	return true
}

// Remaining returns allowance minus used, saturating at zero per dimension.
// used values above allowance clamp to zero remaining (never negative).
func (b Budget) Remaining(used Budget) Budget {
	return Budget{
		ModelCalls: saturatingSubInt(b.ModelCalls, used.ModelCalls),
		Tokens:     saturatingSubInt(b.Tokens, used.Tokens),
		Bytes:      saturatingSubInt64(b.Bytes, used.Bytes),
		Attempts:   saturatingSubInt(b.Attempts, used.Attempts),
		Duration:   saturatingSubDuration(b.Duration, used.Duration),
	}
}

// Add returns the component-wise sum. It does not interpret zero specially.
func (b Budget) Add(other Budget) Budget {
	return Budget{
		ModelCalls: b.ModelCalls + other.ModelCalls,
		Tokens:     b.Tokens + other.Tokens,
		Bytes:      b.Bytes + other.Bytes,
		Attempts:   b.Attempts + other.Attempts,
		Duration:   b.Duration + other.Duration,
	}
}

// Consume returns remaining after debiting cost when b.Covers(cost).
// On insufficient allowance it returns the original budget and an error;
// callers MUST NOT treat a failed consume as success (INV budget monotonicity).
func (b Budget) Consume(cost Budget) (Budget, error) {
	if err := cost.Validate(); err != nil {
		return b, err
	}
	if !b.Covers(cost) {
		return b, errors.New("budget does not cover cost")
	}
	return b.Remaining(cost), nil
}

// Reserve is an alias of Consume for call sites that distinguish reservation
// from final settlement; both are pure and monotonic.
func (b Budget) Reserve(cost Budget) (Budget, error) {
	return b.Consume(cost)
}

// Min returns the component-wise minimum (intersection of two ceilings).
func (b Budget) Min(other Budget) Budget {
	return Budget{
		ModelCalls: minInt(b.ModelCalls, other.ModelCalls),
		Tokens:     minInt(b.Tokens, other.Tokens),
		Bytes:      minInt64(b.Bytes, other.Bytes),
		Attempts:   minInt(b.Attempts, other.Attempts),
		Duration:   minDuration(b.Duration, other.Duration),
	}
}

// Describe returns a short stable summary for audit/reference strings.
func (b Budget) Describe() string {
	return fmt.Sprintf("calls=%d tokens=%d bytes=%d attempts=%d duration=%s",
		b.ModelCalls, b.Tokens, b.Bytes, b.Attempts, b.Duration)
}

func saturatingSubInt(a, b int) int {
	if b >= a {
		return 0
	}
	return a - b
}

func saturatingSubInt64(a, b int64) int64 {
	if b >= a {
		return 0
	}
	return a - b
}

func saturatingSubDuration(a, b time.Duration) time.Duration {
	if b >= a {
		return 0
	}
	return a - b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
