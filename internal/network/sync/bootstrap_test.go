package peersync

import (
	"testing"
)

func TestNewSyncService_NilStore(t *testing.T) {
	svc, err := NewSyncService(nil)
	if err == nil {
		t.Fatalf("expected error when store is nil, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service when store is nil, got %v", svc)
	}
}
