package network

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestP2PManagerDisabled(t *testing.T) {
	manager, err := NewP2PManager(Options{}, nil)
	if err != nil {
		t.Fatalf("NewP2PManager: %v", err)
	}
	if manager != nil {
		t.Fatal("disabled P2P must not construct a manager")
	}
}

func TestP2PManagerLifecycle(t *testing.T) {
	manager, err := NewP2PManager(Options{
		Enabled:  true,
		BindAddr: "127.0.0.1:0",
		NodeID:   "test-node-lifecycle",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewP2PManager failed: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("idempotent Start: %v", err)
	}

	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("idempotent Stop: %v", err)
	}
}

func TestP2PManagerLifecycle_WithMTLSPaths(t *testing.T) {
	dir := t.TempDir()
	pki := generateTestPKI(t, dir)

	manager, err := NewP2PManager(Options{
		Enabled:       true,
		BindAddr:      "127.0.0.1:0",
		NodeID:        "test-node-mtls",
		TLSCertFile:   pki.NodeCert,
		TLSKeyFile:    pki.NodeKey,
		TLSCACertFile: pki.CACert,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewP2PManager failed: %v", err)
	}
	if manager.tlsConfig == nil {
		t.Fatal("expected tlsConfig to be loaded and non-nil")
	}
	if manager.Router == nil || manager.Router.transport == nil {
		t.Fatal("expected router transport to be initialized and non-nil")
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestP2PManagerLifecycle_InvalidMTLSPaths(t *testing.T) {
	_, err := NewP2PManager(Options{
		Enabled:     true,
		BindAddr:    "127.0.0.1:0",
		NodeID:      "test-node-invalid-mtls",
		TLSCertFile: "/invalid/cert.pem",
		TLSKeyFile:  "/invalid/key.pem",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("expected error for invalid mTLS certificate paths, got nil")
	}
}

func TestP2PManagerLifecycle_WithMDNS(t *testing.T) {
	manager, err := NewP2PManager(Options{
		Enabled:     true,
		BindAddr:    "127.0.0.1:0",
		MDNSEnabled: true,
		NodeID:      "test-node-1",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err != nil {
		t.Fatalf("NewP2PManager: %v", err)
	}
	if manager.beacon == nil {
		t.Fatal("beacon should be initialized when MDNSEnabled is true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Multicast may fail on CI so we ignore error during start/stop in this simple config check
	_ = manager.Start(ctx)
	_ = manager.Stop(ctx)
}
