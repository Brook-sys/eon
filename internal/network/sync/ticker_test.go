package peersync

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type mockNetwork struct {
	peers   []domain.PeerRecord
	pullErr error
	pulls   int
}

func (m *mockNetwork) Register(ctx context.Context, peer domain.PeerRecord) error { return nil }
func (m *mockNetwork) Lookup(ctx context.Context, peerID string) (domain.PeerRecord, error) {
	return domain.PeerRecord{}, nil
}
func (m *mockNetwork) List(ctx context.Context) ([]domain.PeerRecord, error) {
	return m.peers, nil
}
func (m *mockNetwork) Evict(ctx context.Context, peerID string) error { return nil }
func (m *mockNetwork) Call(ctx context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return domain.PeerRPCResponse{}, nil
}

type mockService struct {
	err   error
	calls int
}

func (m *mockService) PullOnce(ctx context.Context, caller port.PeerCaller, peerID, localID, streamID string, checkpoint func(PullCheckpoint) error) (PullResult, error) {
	m.calls++
	return PullResult{}, m.err
}

func TestTickerTickExecutesBoundedPullForCapablePeers(t *testing.T) {
	net := &mockNetwork{
		peers: []domain.PeerRecord{
			{Identity: domain.NodeIdentity{ID: "node-a"}, Capabilities: []string{Capability}},
			{Identity: domain.NodeIdentity{ID: "node-missing"}, Capabilities: []string{"other"}},
			{Identity: domain.NodeIdentity{ID: "node-b"}, Capabilities: []string{Capability, "other"}},
		},
	}
	svc := &mockService{}

	ticker, err := NewTicker(svc, net, "node-local", "events", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if err := ticker.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if svc.calls != 2 {
		t.Fatalf("PullOnce calls = %d, want 2 (only capable peers)", svc.calls)
	}
}

func TestTickerTickContinuesDespitePeerFailure(t *testing.T) {
	net := &mockNetwork{
		peers: []domain.PeerRecord{
			{Identity: domain.NodeIdentity{ID: "node-a"}, Capabilities: []string{Capability}},
			{Identity: domain.NodeIdentity{ID: "node-b"}, Capabilities: []string{Capability}},
		},
	}
	someErr := errors.New("timeout")
	svc := &mockService{err: someErr}

	ticker, _ := NewTicker(svc, net, "node-local", "events", time.Minute)
	err := ticker.Tick(context.Background())
	if !errors.Is(err, someErr) {
		t.Fatalf("want %v, got %v", someErr, err)
	}
	if svc.calls != 2 {
		t.Fatalf("PullOnce calls = %d, want 2 (should not stop on first error)", svc.calls)
	}
}

func TestTickerRejectsInvalidConfig(t *testing.T) {
	net := &mockNetwork{}
	svc := &mockService{}
	for _, tc := range []struct {
		s        puller
		n        port.Network
		localID  string
		streamID string
		interval time.Duration
	}{
		{nil, net, "local", "stream", time.Second},
		{svc, nil, "local", "stream", time.Second},
		{svc, net, "", "stream", time.Second},
		{svc, net, "local", "", time.Second},
		{svc, net, "local", "stream", 0},
	} {
		if _, err := NewTicker(tc.s, tc.n, tc.localID, tc.streamID, tc.interval); !errors.Is(err, ErrInvalidTicker) {
			t.Fatalf("expected ErrInvalidTicker, got %v", err)
		}
	}
}
