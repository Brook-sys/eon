package domain

import (
	"testing"
)

func TestSQLInterfaces(t *testing.T) {
	budget := Budget{Tokens: 100}
	val, err := budget.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bytes, ok := val.([]byte)
	if !ok {
		t.Fatalf("expected []byte from Budget.Value()")
	}

	var scanned Budget
	if err := scanned.Scan(bytes); err != nil {
		t.Fatalf("unexpected error during Scan: %v", err)
	}

	if scanned.Tokens != 100 {
		t.Errorf("expected Tokens 100, got %d", scanned.Tokens)
	}

	// Test string scanning
	var scannedString Budget
	if err := scannedString.Scan(string(bytes)); err != nil {
		t.Fatalf("unexpected error during Scan (string): %v", err)
	}
	if scannedString.Tokens != 100 {
		t.Errorf("expected Tokens 100, got %d", scannedString.Tokens)
	}
}
