package domain

import (
	"testing"
	"time"
)

func TestBudgetCoversAndConsume(t *testing.T) {
	allowance := Budget{ModelCalls: 3, Tokens: 1000, Bytes: 4096, Attempts: 2, Duration: time.Minute}
	cost := Budget{ModelCalls: 1, Tokens: 200, Attempts: 1}
	if !allowance.Covers(cost) {
		t.Fatal("expected cover")
	}
	remaining, err := allowance.Consume(cost)
	if err != nil {
		t.Fatal(err)
	}
	if remaining.ModelCalls != 2 || remaining.Tokens != 800 || remaining.Attempts != 1 {
		t.Fatalf("remaining = %+v", remaining)
	}
	// Zero cost always covers.
	var zero Budget
	if !zero.Covers(Budget{}) {
		t.Fatal("zero covers zero")
	}
	// Zero allowance never covers positive cost.
	if zero.Covers(Budget{ModelCalls: 1}) {
		t.Fatal("zero must not cover positive calls")
	}
	if _, err := remaining.Consume(Budget{ModelCalls: 5}); err == nil {
		t.Fatal("expected insufficient budget error")
	}
}

func TestBudgetRemainingSaturates(t *testing.T) {
	b := Budget{ModelCalls: 2, Tokens: 10}
	used := Budget{ModelCalls: 5, Tokens: 3}
	r := b.Remaining(used)
	if r.ModelCalls != 0 || r.Tokens != 7 {
		t.Fatalf("remaining = %+v", r)
	}
}

func TestBudgetMinAndAdd(t *testing.T) {
	a := Budget{ModelCalls: 5, Tokens: 100}
	b := Budget{ModelCalls: 2, Tokens: 500, Attempts: 1}
	m := a.Min(b)
	if m.ModelCalls != 2 || m.Tokens != 100 || m.Attempts != 0 {
		t.Fatalf("min = %+v", m)
	}
	s := a.Add(b)
	if s.ModelCalls != 7 || s.Tokens != 600 || s.Attempts != 1 {
		t.Fatalf("sum = %+v", s)
	}
}

func TestBudgetZeroMeansNone(t *testing.T) {
	// Documented semantics: zero is not unlimited.
	specish := Budget{Tokens: 1000} // ModelCalls zero
	if specish.Covers(Budget{ModelCalls: 1, Tokens: 1}) {
		t.Fatal("zero model_calls must not authorize a call")
	}
	var empty Budget
	if !empty.IsZero() {
		t.Fatal("empty budget must be IsZero")
	}
	if specish.IsZero() {
		t.Fatal("tokens-only budget must not be IsZero")
	}
}
