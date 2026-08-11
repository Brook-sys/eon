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
	service       puller
	canonicalizer InboxCanonicalizer   // Optional processor for inbox events
	retention     *RetentionReporter   // Optional post-sync pressure observer
	network       port.Network
	localID       string
	streamID      string
	interval      time.Duration
}

// ReconcilePeerNow triggers an on-demand sync pull against a specific peer outside the background ticker cycle.
func (t *Ticker) ReconcilePeerNow(ctx context.Context, peerID, localID, streamID string) (PullResult, error) {
	if t.service == nil {
		return PullResult{}, ErrInvalidTicker
	}
	res, err := t.service.PullOnce(ctx, t.network, peerID, localID, streamID, nil)
	if err != nil {
		return PullResult{}, err
	}
	if t.canonicalizer != nil {
		_, _ = t.canonicalizer.Reconcile(ctx, peerID)
	}
	return res, nil
}

// AttachCanonicalizer registers the inbox-to-canonical reconciler which runs immediately after PullOnce.
func (t *Ticker) AttachCanonicalizer(c InboxCanonicalizer) {
	t.canonicalizer = c
}

// AttachRetentionReporter registers a post-sync retention pressure observer.
func (t *Ticker) AttachRetentionReporter(r *RetentionReporter) {
	t.retention = r
}

// LatestRetentionReport returns the current pressure snapshot, if a reporter is attached.
func (t *Ticker) LatestRetentionReport(ctx context.Context) (RetentionPressureReport, error) {
	if t.retention == nil {
		return RetentionPressureReport{}, nil
	}
	return t.retention.Report(ctx)
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
		// Optional canonicalizer drains the inbox safely if wired.
		if t.canonicalizer != nil {
			_, _ = t.canonicalizer.Reconcile(ctx, peer.Identity.ID)
		}
	}
	// Post-sync: observe retention pressure (read-only, never mutates).
	if t.retention != nil {
		_, _ = t.retention.Report(ctx)
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
