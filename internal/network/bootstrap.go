package network

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	peerhttp "motor-autonomo/internal/network/http"
	"motor-autonomo/internal/network/mdns"
	peersync "motor-autonomo/internal/network/sync"
)

// Options define as flags e politicas para o subsistema de rede P2P.
type Options struct {
	// Enabled habilita a abertura da porta mTLS local.
	Enabled bool
	// BindAddr e o endereco TCP para ouvir conexoes P2P, por exemplo ":8443".
	BindAddr string
	// Certificados obrigatorios para mTLS.
	TLSCertFile   string
	TLSKeyFile    string
	TLSCACertFile string
	// MDNSEnabled habilita o beacon mDNS.
	MDNSEnabled bool
	NodeID      string
}

// P2PManager agrupa os recursos do subsistema de rede que precisam de start/stop.
type P2PManager struct {
	Router     *Router
	Registry   *StaticRegistry
	Ticker     *peersync.Ticker
	httpServer *http.Server
	beacon     *mdns.Beacon
	listenAddr string
	tlsConfig  *tls.Config
	logger     *slog.Logger
	cancelTick context.CancelFunc
	mu         sync.Mutex
	running    bool
}

// NewP2PManager inicializa os dominios P2P e o handler HTTP.
// Apenas inicia os servicos de fato ao chamar Start.
func NewP2PManager(opts Options, logger *slog.Logger) (*P2PManager, error) {
	if !opts.Enabled {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	if opts.BindAddr == "" {
		opts.BindAddr = "127.0.0.1:8443"
	}
	if opts.NodeID == "" {
		return nil, fmt.Errorf("network: NodeID is required; deterministic callers must set it explicitly")
	}

	registry, err := NewStaticRegistry(domain.PeerRegistryPolicy{
		MaxPeers:        100,
		EvictionTimeout: 5 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("create registry: %w", err)
	}

	// Transport dummy para mock isolado sem pki, apenas satisfaz a validacao do Router
	dummyTransport := &peerhttp.Transport{}
	router, err := NewRouter(registry, dummyTransport)
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	if err := router.SetLocalID(opts.NodeID); err != nil {
		return nil, fmt.Errorf("set router local id: %w", err)
	}

	// peerhttp.ServerHandler assume PeerCaller, Router implementa PeerCaller
	handler, err := peerhttp.NewServerHandler(router)
	if err != nil {
		return nil, fmt.Errorf("create handler: %w", err)
	}

	srv := &http.Server{
		Addr:              opts.BindAddr,
		Handler:           handler,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	var beacon *mdns.Beacon
	if opts.MDNSEnabled {
		// Extract port from BindAddr if possible
		portNum := 8443
		_, pStr, err := net.SplitHostPort(opts.BindAddr)
		if err == nil {
			p, _ := strconv.Atoi(pStr)
			if p > 0 {
				portNum = p
			}
		}

		mdnsCfg := mdns.MDNSConfig{
			NodeID:            opts.NodeID,
			Port:              portNum,
			AdvertiseInterval: 30 * time.Second,
		}
		beacon, err = mdns.NewBeacon(mdnsCfg, registry)
		if err != nil {
			return nil, fmt.Errorf("create mdns beacon: %w", err)
		}
	}

	var tlsConfig *tls.Config
	if opts.TLSCertFile != "" && opts.TLSKeyFile != "" {
		caCert := opts.TLSCACertFile
		if caCert == "" {
			caCert = opts.TLSCertFile
		}
		cfg := PeerConfig{
			NodeCert: opts.TLSCertFile,
			NodeKey:  opts.TLSKeyFile,
			CACert:   caCert,
		}
		tc, err := LoadMTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("load mtls config: %w", err)
		}
		tlsConfig = tc

		transport, err := peerhttp.NewTransport(tlsConfig, 30*time.Second)
		if err != nil {
			return nil, fmt.Errorf("create peer transport: %w", err)
		}
		if err := router.SetTransport(transport); err != nil {
			return nil, fmt.Errorf("set router transport: %w", err)
		}
	}

	return &P2PManager{
		Router:     router,
		Registry:   registry,
		httpServer: srv,
		beacon:     beacon,
		listenAddr: opts.BindAddr,
		tlsConfig:  tlsConfig,
		logger:     logger,
	}, nil
}

// Start abre o listener P2P. A integracao produtiva deve usar exclusivamente
// a configuracao TLS/mTLS estrita do pacote network/http.
func (m *P2PManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}

	ln, err := net.Listen("tcp", m.listenAddr)
	if err != nil {
		return fmt.Errorf("listen P2P address: %w", err)
	}

	if m.tlsConfig != nil {
		ln = tls.NewListener(ln, m.tlsConfig)
	}

	m.logger.InfoContext(ctx, "starting P2P server", slog.String("addr", ln.Addr().String()))

	if m.beacon != nil {
		if err := m.beacon.Start(ctx); err != nil {
			ln.Close()
			return fmt.Errorf("start mdns beacon: %w", err)
		}
		m.logger.InfoContext(ctx, "started mDNS beacon")
	}

	m.running = true
	if m.Ticker != nil {
		tickCtx, cancel := context.WithCancel(context.Background())
		m.cancelTick = cancel
		go func() {
			_ = m.Ticker.Run(tickCtx)
		}()
	}
	go func() {
		if err := m.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			m.logger.Error("P2P server failed", slog.String("error", err.Error()))
		}
	}()
	return nil
}

// Stop executa shutdown gracioso do servidor P2P.
func (m *P2PManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return nil
	}

	if m.cancelTick != nil {
		m.cancelTick()
	}

	if m.beacon != nil {
		m.beacon.Stop()
	}

	err := m.httpServer.Shutdown(ctx)
	m.running = false
	return err
}
