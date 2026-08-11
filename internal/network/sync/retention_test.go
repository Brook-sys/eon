package peersync

import (
	"context"
	"motor-autonomo/internal/domain"
	"testing"
)

type mockDeleter struct {
	deleted [][]byte
}

func (m *mockDeleter) DeleteEvent(ctx context.Context, id []byte) error {
	m.deleted = append(m.deleted, id)
	return nil
}

func TestDeleteStaleEventsUnauthorized(t *testing.T) {
	policy := domain.DefaultStoreRetentionPolicy() // MVP default has AllowEventLogPrune = false
	deleter := &mockDeleter{}

	err := DeleteStaleEvents(context.Background(), policy, deleter)
	if err != domain.ErrUnauthorizedRetention {
		t.Fatalf("expected ErrUnauthorizedRetention, got %v", err)
	}
	if len(deleter.deleted) != 0 {
		t.Fatalf("deleter was called despite unauthorized policy")
	}
}

func TestDeleteStaleEventsAuthorized(t *testing.T) {
	policy := domain.DefaultStoreRetentionPolicy()
	policy.AllowEventLogPrune = true
	deleter := &mockDeleter{}

	err := DeleteStaleEvents(context.Background(), policy, deleter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Currently a stub so nothing is actually deleted yet
}
