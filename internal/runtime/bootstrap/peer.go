package bootstrap

import (
	"fmt"
	"time"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/network"
	"motor-autonomo/internal/network/http"
)

func buildPeerTransport(opts Options) (*kernel.PeerTransport, error) {
	if opts.PeerBindAddr == "" {
		return nil, nil // Disabled by default
	}

	cfg := network.PeerConfig{
		NodeCert: opts.PeerCert,
		NodeKey:  opts.PeerKey,
		CACert:   opts.PeerCACert,
	}
	tlsConfig, err := network.LoadMTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("load peer mtls: %w", err)
	}

	policy := domain.PeerRegistryPolicy{
		MaxPeers: 10,
	}
	registry, err := network.NewStaticRegistry(policy)
	if err != nil {
		return nil, fmt.Errorf("init peer registry: %w", err)
	}

	transport, err := peerhttp.NewTransport(tlsConfig, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("init peer transport: %w", err)
	}

	// Router implements PeerCaller given a Registry and PeerTransport.
	caller, err := network.NewRouter(registry, transport)
	if err != nil {
		return nil, fmt.Errorf("init peer router: %w", err)
	}

	// Combine into PeerTransport kernel bundle.
	return kernel.NewPeerTransport(registry, caller), nil
}
