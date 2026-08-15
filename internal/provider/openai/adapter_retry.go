package openai

import (
	"time"

	"motor-autonomo/internal/runtime/source"
)

// RetryConfig configures bounded retry with exponential backoff for retryable
// provider errors (HTTP 429 and 5xx). It is optional: when nil, retries are
// disabled and the adapter returns the first error (backward compatible).
type RetryConfig struct {
	// MaxAttempts is the total number of attempts including the first.
	// Must be >= 1. A value of 1 disables retries (single attempt).
	MaxAttempts int
	// BaseDelay is the initial backoff delay before the first retry.
	// Must be > 0 when MaxAttempts > 1.
	BaseDelay time.Duration
	// MaxDelay caps the exponential backoff. Must be >= BaseDelay.
	MaxDelay time.Duration
	// MaxJitter adds random jitter to each delay to avoid thundering herd.
	// Zero disables jitter. When > 0, a crypto random source is used.
	MaxJitter time.Duration
}

// cryptoJitterSource implements retry.JitterSource using the project's
// CryptoRandomSource for cryptographically secure retry jitter.
type cryptoJitterSource struct{}

func (cryptoJitterSource) Uint64() (uint64, error) {
	return source.CryptoRandomSource{}.Uint64()
}
