package peersync

import (
	"context"
	"errors"
	"time"

	"motor-autonomo/internal/port"
)

var ErrInvalidTicker = errors.New("invalid peer sync ticker configuration")

type puller interface {
	PullOnce(context.Context, port.PeerCaller, string, string, string, func(PullCheckpoint) error) (PullResult, error)
}

// Ticker performs one bounded pull per discovered peer on each mesh tick.
// It never drains in a tight loop: durable cursors provide forward progress
// across ticks and process restarts.
type Ticker struct {
	service  puller
	network  port.Network
	localID  string
	streamID string
	interval time.Duration
}

func NewTicker(service puller, network port.Network, localID, streamID string, interval time.Duration) (*Ticker, error) {
	if service == nil || network == nil || !validSyncID(localID) || !validSyncID(streamID) || interval <= 0 {
		return nil, ErrInvalidTicker
	}
	return &Ticker{service: service, network: network, localID: localID, streamID: streamID, interval: interval}, nil
}

// Tick performs a single bounded mesh pass. One peer failure does not prevent
// healthy peers from advancing; the first failure is returned for telemetry.
func (t *Ticker) Tick(ctx context.Context) error {
	peers, err := t.network.List(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, peer := range peers {
		if !hasSyncCapability(peer.Capabilities) {
			continue
		}
		if _, err := t.service.PullOnce(ctx, t.network, peer.Identity.ID, t.localID, t.streamID, nil); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Run starts the mesh tick loop and exits cleanly when ctx is cancelled.
func (t *Ticker) Run(ctx context.Context) error {
	clock := time.NewTicker(t.interval)
	defer clock.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-clock.C:
			_ = t.Tick(ctx)
		}
	}
}

func hasSyncCapability(capabilities []string) bool {
	for _, capability := range capabilities {
		if capability == Capability {
			return true
		}
	}
	return false
}
