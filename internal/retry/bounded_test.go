package retry_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"motor-autonomo/internal/retry"
	"motor-autonomo/internal/runtime/source"
)

var (
	errBusy     = errors.New("busy")
	errConflict = errors.New("conflict")
	errFatal    = errors.New("fatal")
)

type recordingSleeper struct {
	delays []time.Duration
	err    error
}

func (s *recordingSleeper) Sleep(_ context.Context, delay time.Duration) error {
	s.delays = append(s.delays, delay)
	return s.err
}

type cancelingSleeper struct {
	cancel context.CancelFunc
}

func (s *cancelingSleeper) Sleep(ctx context.Context, _ time.Duration) error {
	s.cancel()
	return ctx.Err()
}

func classify(err error) (string, bool) {
	switch {
	case errors.Is(err, errBusy):
		return "busy", true
	case errors.Is(err, errConflict):
		return "conflict", true
	default:
		return "fatal", false
	}
}

func TestDoRetriesBoundedlyWithCappedBackoffJitterAndMetrics(t *testing.T) {
	sleeper := &recordingSleeper{}
	jitter := source.NewSequenceRandomSource(uint64(3*time.Millisecond), uint64(2*time.Millisecond))
	errorsByAttempt := []error{errBusy, errConflict, nil}
	report, err := retry.Do(context.Background(), retry.Policy{
		MaxAttempts: 4,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    15 * time.Millisecond,
		MaxJitter:   5 * time.Millisecond,
	}, sleeper, jitter, classify, func(_ context.Context, attempt int) error {
		return errorsByAttempt[attempt-1]
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDelays := []time.Duration{13 * time.Millisecond, 17 * time.Millisecond}
	if !reflect.DeepEqual(sleeper.delays, wantDelays) {
		t.Fatalf("delays = %v, want %v", sleeper.delays, wantDelays)
	}
	if report.Attempts != 3 || report.Retries != 2 || report.SleepTotal != 30*time.Millisecond {
		t.Fatalf("report = %+v", report)
	}
	if !reflect.DeepEqual(report.Classes, map[string]int{"busy": 1, "conflict": 1}) {
		t.Fatalf("classes = %#v", report.Classes)
	}
}

func TestDoStopsImmediatelyOnNonRetryableError(t *testing.T) {
	sleeper := &recordingSleeper{}
	report, err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 4}, sleeper, nil, classify, func(context.Context, int) error {
		return errFatal
	})
	if !errors.Is(err, errFatal) {
		t.Fatalf("error = %v, want fatal", err)
	}
	if report.Attempts != 1 || report.Retries != 0 || len(sleeper.delays) != 0 || report.Classes["fatal"] != 1 {
		t.Fatalf("report = %+v delays=%v", report, sleeper.delays)
	}
}

func TestDoExhaustionPreservesLastError(t *testing.T) {
	report, err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 2, BaseDelay: time.Second, MaxDelay: time.Second}, &recordingSleeper{}, nil, classify, func(context.Context, int) error {
		return errBusy
	})
	if !errors.Is(err, retry.ErrBudgetExhausted) || !errors.Is(err, errBusy) {
		t.Fatalf("error = %v, want budget exhausted joined with busy", err)
	}
	if report.Attempts != 2 || report.Retries != 1 || report.Classes["busy"] != 2 {
		t.Fatalf("report = %+v", report)
	}
}

func TestDoCancellationBeforeFirstAttemptStopsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	report, err := retry.Do(ctx, retry.Policy{MaxAttempts: 2}, &recordingSleeper{}, nil, classify, func(context.Context, int) error {
		calls++
		return errBusy
	})
	if !errors.Is(err, context.Canceled) || calls != 0 || report.Attempts != 0 {
		t.Fatalf("err=%v calls=%d report=%+v", err, calls, report)
	}
}

func TestDoCancellationDuringSleepStopsBeforeNextAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sleeper := &cancelingSleeper{cancel: cancel}
	calls := 0
	report, err := retry.Do(ctx, retry.Policy{MaxAttempts: 2, BaseDelay: time.Second, MaxDelay: time.Second}, sleeper, nil, classify, func(context.Context, int) error {
		calls++
		return errBusy
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || report.Attempts != 1 || report.Retries != 1 {
		t.Fatalf("err=%v calls=%d report=%+v", err, calls, report)
	}
}

func TestDoRejectsUnboundedOrIncompletePolicy(t *testing.T) {
	cases := []struct {
		name   string
		policy retry.Policy
		sleep  retry.Sleeper
		jitter retry.JitterSource
	}{
		{name: "zero attempts", policy: retry.Policy{}, sleep: &recordingSleeper{}},
		{name: "negative delay", policy: retry.Policy{MaxAttempts: 1, BaseDelay: -1}, sleep: &recordingSleeper{}},
		{name: "missing cap", policy: retry.Policy{MaxAttempts: 1, BaseDelay: 1}, sleep: &recordingSleeper{}},
		{name: "base beyond cap", policy: retry.Policy{MaxAttempts: 1, BaseDelay: 2, MaxDelay: 1}, sleep: &recordingSleeper{}},
		{name: "missing sleeper", policy: retry.Policy{MaxAttempts: 1}},
		{name: "missing jitter", policy: retry.Policy{MaxAttempts: 1, MaxJitter: 1}, sleep: &recordingSleeper{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := retry.Do(context.Background(), tc.policy, tc.sleep, tc.jitter, classify, func(context.Context, int) error { return nil }); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
