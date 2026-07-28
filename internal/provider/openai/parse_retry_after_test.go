package openai

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterDelaySeconds(t *testing.T) {
	cases := []struct {
		value string
		want  time.Duration
	}{
		{"0", 0},
		{"1", 1 * time.Second},
		{"42", 42 * time.Second},
		{"  10  ", 10 * time.Second},
		{"999999", 999999 * time.Second},
	}
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		got := parseRetryAfter(tc.value, now)
		if got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	// Future HTTP-date → positive duration.
	future := now.Add(30 * time.Second)
	header := future.UTC().Format(http.TimeFormat)
	got := parseRetryAfter(header, now)
	if got <= 0 || got > 30*time.Second+1*time.Second {
		t.Fatalf("parseRetryAfter(http-date future) = %v, want ~30s", got)
	}
}

func TestParseRetryAfterPastHTTPDateFailsClosed(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	past := now.Add(-5 * time.Minute)
	header := past.UTC().Format(http.TimeFormat)
	got := parseRetryAfter(header, now)
	if got != 0 {
		t.Fatalf("parseRetryAfter(past http-date) = %v, want 0 (fail closed)", got)
	}
}

func TestParseRetryAfterEmpty(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	if got := parseRetryAfter("", now); got != 0 {
		t.Fatalf("parseRetryAfter(\"\") = %v, want 0", got)
	}
	if got := parseRetryAfter("   ", now); got != 0 {
		t.Fatalf("parseRetryAfter(\"   \") = %v, want 0", got)
	}
}

func TestParseRetryAfterMalformed(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	invalid := []string{
		"not-a-duration",
		"abc",
		"12x",
		"Mon, 99 Jan 9999 99:99:99 GMT",
	}
	for _, v := range invalid {
		got := parseRetryAfter(v, now)
		if got != 0 {
			t.Errorf("parseRetryAfter(%q) = %v, want 0 (fail closed)", v, got)
		}
	}
}

func TestParseRetryAfterNegativeDelayFailsClosed(t *testing.T) {
	// "ParseDuration" won't accept bare "-5" as seconds because the "s" suffix
	// is prepended by the function: "-5s". The delay-seconds form in RFC 9110
	// is non-negative, but test that a negative-ish value is rejected.
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	got := parseRetryAfter("-5", now)
	if got != 0 {
		t.Fatalf("parseRetryAfter(\"-5\") = %v, want 0 (fail closed)", got)
	}
}

func TestParseRetryAfterBoundaryNow(t *testing.T) {
	// HTTP-date exactly equal to now is not strictly "After(now)", so the
	// function must fail closed to zero.
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	header := now.UTC().Format(http.TimeFormat)
	got := parseRetryAfter(header, now)
	if got != 0 {
		t.Fatalf("parseRetryAfter(http-date == now) = %v, want 0", got)
	}
}

func TestParseRetryAfterOneSecondBeforeNow(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	justBefore := now.Add(-1 * time.Second)
	header := justBefore.UTC().Format(http.TimeFormat)
	got := parseRetryAfter(header, now)
	if got != 0 {
		t.Fatalf("parseRetryAfter(http-date 1s before now) = %v, want 0", got)
	}
}

func TestParseRetryAfterOneSecondAfterNow(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	justAfter := now.Add(1 * time.Second)
	header := justAfter.UTC().Format(http.TimeFormat)
	got := parseRetryAfter(header, now)
	if got <= 0 || got > 1*time.Second {
		t.Fatalf("parseRetryAfter(http-date 1s after now) = %v, want (0,1s]", got)
	}
}

func TestParseRetryAfterDeltaSecondsPreferenceOverDate(t *testing.T) {
	// The function first tries delay-seconds; a valid delay must win even if
	// the value also happens to parse as an HTTP-date (unlikely but the
	// branch order is deliberate).
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	// "5" is a valid delay-seconds value → 5s.
	got := parseRetryAfter("5", now)
	if got != 5*time.Second {
		t.Fatalf("parseRetryAfter(\"5\") = %v, want 5s", got)
	}
}

func TestParseRetryAfterHTTPGMTOnly(t *testing.T) {
	// RFC 9110 HTTP-date is always GMT. A non-GMT timezone in the date
	// string should fail via http.ParseTime (which requires GMT).
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	// Build a date string without GMT suffix — http.ParseTime should reject.
	bad := "Mon, 28 Jul 2026 10:00:05 PST"
	got := parseRetryAfter(bad, now)
	if got != 0 {
		t.Fatalf("parseRetryAfter(non-GMT date) = %v, want 0", got)
	}
}

func TestParseRetryAfterSummary(t *testing.T) {
	// Quick summary table for documentation reference.
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		input string
		want  time.Duration
		desc  string
	}{
		{"", 0, "empty → fail closed"},
		{"42", 42 * time.Second, "delay-seconds"},
		{now.Add(-time.Hour).UTC().Format(http.TimeFormat), 0, "past HTTP-date → fail closed"},
		{now.Add(10 * time.Second).UTC().Format(http.TimeFormat), 10 * time.Second, "future HTTP-date → positive"},
		{"garbage", 0, "invalid → fail closed"},
	}
	for _, tc := range cases {
		got := parseRetryAfter(tc.input, now)
		descOK := true
		if tc.want == 0 && got != 0 {
			descOK = false
		}
		if tc.want > 0 && (got <= 0 || got > tc.want+time.Second) {
			descOK = false
		}
		if !descOK {
			t.Errorf("parseRetryAfter(%q) [%s] = %v, expected ~%v", tc.input, tc.desc, got, tc.want)
		}
	}
}
