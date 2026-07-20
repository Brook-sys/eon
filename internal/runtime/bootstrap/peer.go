package bootstrap

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/network"
	peerhttp "motor-autonomo/internal/network/http"
	"motor-autonomo/internal/network/subagentspawn"
	"motor-autonomo/internal/network/subagentstatus"
	peersync "motor-autonomo/internal/network/sync"
	"motor-autonomo/internal/port"
)

func buildPeerTransport(opts Options, store port.Store, now func() time.Time, sessions kernel.SessionManager) (*kernel.PeerTransport, error) {
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
	if opts.PeerNodeID == "" {
		// Existing deployments did not expose an explicit node-id flag. Derive a
		// stable local identity from the certificate SAN while keeping peer auth
		// anchored in mTLS.
		certificatePEM, loadErr := os.ReadFile(opts.PeerCert)
		if loadErr != nil {
			return nil, fmt.Errorf("derive peer node id: %w", loadErr)
		}
		block, _ := pem.Decode(certificatePEM)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("derive peer node id: invalid certificate PEM")
		}
		certificate, loadErr := x509.ParseCertificate(block.Bytes)
		if loadErr != nil {
			return nil, fmt.Errorf("derive peer node id: %w", loadErr)
		}
		opts.PeerNodeID, loadErr = peerhttp.PeerIDFromCertificate(certificate)
		if loadErr != nil {
			return nil, fmt.Errorf("derive peer node id: %w", loadErr)
		}
	}
	syncService, err := peersync.NewService(store, now)
	if err != nil {
		return nil, fmt.Errorf("init peer sync: %w", err)
	}
	if err := caller.AttachSync(opts.PeerNodeID, syncService); err != nil {
		return nil, fmt.Errorf("attach peer sync: %w", err)
	}
	if sessions != nil {
		statusService, statusErr := subagentstatus.NewService(sessions)
		if statusErr != nil {
			return nil, fmt.Errorf("init subagent status ingress: %w", statusErr)
		}
		if statusErr := caller.AttachSubagentStatuses(statusService); statusErr != nil {
			return nil, fmt.Errorf("attach subagent status ingress: %w", statusErr)
		}
		spawnService, spawnErr := subagentspawn.NewService(sessions)
		if spawnErr != nil {
			return nil, fmt.Errorf("init subagent spawn ingress: %w", spawnErr)
		}
		if spawnErr := caller.AttachSubagentSpawns(spawnService); spawnErr != nil {
			return nil, fmt.Errorf("attach subagent spawn ingress: %w", spawnErr)
		}
	}

	// Combine into PeerTransport kernel bundle and attach the bounded mesh tick.
	bundle := kernel.NewPeerTransport(registry, caller)
	interval := opts.PeerSyncInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	bundle.Sync, err = peersync.NewTicker(syncService, caller, opts.PeerNodeID, "events", interval)
	if err != nil {
		return nil, fmt.Errorf("init peer sync ticker: %w", err)
	}

	canonicalizer, err := peersync.NewBoundedInboxCanonicalizer(store, peersync.NewBasicConflictResolver())
	if err != nil {
		return nil, fmt.Errorf("init inbox canonicalizer: %w", err)
	}
	bundle.Sync.AttachCanonicalizer(canonicalizer)
	return bundle, nil
}
