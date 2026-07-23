// Package retry provides explicit, bounded retry orchestration for callers that
// have already established that an operation is safe to repeat. It does not
// infer idempotency and deliberately stays outside storage adapters.
package retry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrBudgetExhausted = errors.New("retry budget exhausted")

type Sleeper interface {
	Sleep(context.Context, time.Duration) error
}

type JitterSource interface {
	Uint64() (uint64, error)
}

type Classifier func(error) (class string, retryable bool)

type Policy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	MaxJitter   time.Duration
}

type Report struct {
	Attempts   int
	Retries    int
	SleepTotal time.Duration
	Classes    map[string]int
}

type SystemSleeper struct{}

func (SystemSleeper) Sleep(ctx context.Context, delay time.Duration) error {
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

// Do invokes operation at most policy.MaxAttempts times. The caller supplies
// the complete retry classifier and must ensure operation is idempotent. A
// retryable final failure is joined with ErrBudgetExhausted so the original
// error remains inspectable with errors.Is/errors.As.
func Do(ctx context.Context, policy Policy, sleeper Sleeper, jitter JitterSource, classify Classifier, operation func(context.Context, int) error) (Report, error) {
	report := Report{Classes: make(map[string]int)}
	if err := validate(policy, sleeper, jitter, classify, operation); err != nil {
		return report, err
	}
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		report.Attempts = attempt
		err := operation(ctx, attempt)
		if err == nil {
			return report, nil
		}
		class, retryable := classify(err)
		if class != "" {
			report.Classes[class]++
		}
		if !retryable {
			return report, err
		}
		if attempt == policy.MaxAttempts {
			return report, errors.Join(ErrBudgetExhausted, err)
		}
		delay, err := delayFor(policy, attempt, jitter)
		if err != nil {
			return report, fmt.Errorf("retry jitter: %w", err)
		}
		report.Retries++
		report.SleepTotal += delay
		if err := sleeper.Sleep(ctx, delay); err != nil {
			return report, err
		}
	}
	panic("unreachable")
}

func validate(policy Policy, sleeper Sleeper, jitter JitterSource, classify Classifier, operation func(context.Context, int) error) error {
	if policy.MaxAttempts < 1 {
		return errors.New("retry max attempts must be positive")
	}
	if policy.BaseDelay < 0 || policy.MaxDelay < 0 || policy.MaxJitter < 0 {
		return errors.New("retry delays must not be negative")
	}
	if policy.BaseDelay > 0 && policy.MaxDelay == 0 {
		return errors.New("retry maximum delay is required when backoff is enabled")
	}
	if policy.MaxDelay > 0 && policy.BaseDelay > policy.MaxDelay {
		return errors.New("retry base delay exceeds maximum delay")
	}
	if sleeper == nil || classify == nil || operation == nil {
		return errors.New("retry sleeper, classifier, and operation are required")
	}
	if policy.MaxJitter > 0 && jitter == nil {
		return errors.New("retry jitter source is required when jitter is enabled")
	}
	return nil
}

func delayFor(policy Policy, retryNumber int, jitter JitterSource) (time.Duration, error) {
	delay := policy.BaseDelay
	for step := 1; step < retryNumber; step++ {
		if policy.MaxDelay > 0 && delay >= policy.MaxDelay/2 {
			delay = policy.MaxDelay
			break
		}
		delay *= 2
	}
	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}
	if policy.MaxJitter > 0 {
		value, err := jitter.Uint64()
		if err != nil {
			return 0, err
		}
		// Durations are signed; constraining the modulus first avoids overflow.
		delay += time.Duration(value % uint64(policy.MaxJitter+1))
	}
	return delay, nil
}
