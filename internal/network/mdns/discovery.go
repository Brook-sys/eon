package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"

	"github.com/miekg/dns"
)

var (
	ErrInvalidConfig   = errors.New("invalid mdns configuration")
	ErrDiscoveryFailed = errors.New("mdns discovery failed")
)

const (
	defaultMulticastAddress = "224.0.0.251:5353"
	txtPrefix               = "v=openclaw-p2p-1"
	mdnsService             = "_openclaw._tcp.local."
)

type MDNSConfig struct {
	BindAddress       string
	MulticastAddress  string
	NodeID            string
	AllowedPKIHashes  []string
	AdvertiseInterval time.Duration
	Port              int
}

type Beacon struct {
	config   MDNSConfig
	registry port.PeerRegistry

	mu      sync.Mutex
	conn    *net.UDPConn
	running bool
}

func NewBeacon(config MDNSConfig, registry port.PeerRegistry) (*Beacon, error) {
	if config.NodeID == "" || registry == nil {
		return nil, ErrInvalidConfig
	}
	if config.MulticastAddress == "" {
		config.MulticastAddress = defaultMulticastAddress
	}
	if config.AdvertiseInterval == 0 {
		config.AdvertiseInterval = 30 * time.Second
	}

	return &Beacon{
		config:   config,
		registry: registry,
	}, nil
}

func (b *Beacon) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return nil
	}

	addr, err := net.ResolveUDPAddr("udp4", b.config.MulticastAddress)
	if err != nil {
		b.mu.Unlock()
		return fmt.Errorf("%w: resolve addr: %v", ErrDiscoveryFailed, err)
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		b.mu.Unlock()
		return fmt.Errorf("%w: listen: %v", ErrDiscoveryFailed, err)
	}

	b.conn = conn
	b.running = true
	b.mu.Unlock()

	go b.listen(ctx)
	go b.advertise(ctx)

	return nil
}

func (b *Beacon) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return nil
	}
	b.running = false
	if b.conn != nil {
		return b.conn.Close()
	}
	return nil
}

func (b *Beacon) advertise(ctx context.Context) {
	ticker := time.NewTicker(b.config.AdvertiseInterval)
	defer ticker.Stop()

	b.sendAdvertisement()

	for {
		select {
		case <-ctx.Done():
			b.Stop()
			return
		case <-ticker.C:
			b.sendAdvertisement()
		}
	}
}

func (b *Beacon) sendAdvertisement() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running || b.conn == nil {
		return
	}

	m := new(dns.Msg)
	m.Id = dns.Id()
	m.Response = true
	m.Authoritative = true

	srvName := fmt.Sprintf("%s.%s", b.config.NodeID, mdnsService)

	m.Answer = append(m.Answer, &dns.PTR{
		Hdr: dns.RR_Header{Name: mdnsService, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 120},
		Ptr: srvName,
	})

	m.Extra = append(m.Extra, &dns.SRV{
		Hdr:    dns.RR_Header{Name: srvName, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 120},
		Target: srvName,
		Port:   uint16(b.config.Port),
	})
	m.Extra = append(m.Extra, &dns.TXT{
		Hdr: dns.RR_Header{Name: srvName, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 120},
		Txt: []string{
			txtPrefix,
			fmt.Sprintf("id=%s", b.config.NodeID),
		},
	})

	buf, err := m.Pack()
	if err != nil {
		return
	}

	addr, _ := net.ResolveUDPAddr("udp4", b.config.MulticastAddress)
	_, _ = b.conn.WriteToUDP(buf, addr)
}

func (b *Beacon) listen(ctx context.Context) {
	buf := make([]byte, 9000)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		b.mu.Lock()
		conn := b.conn
		running := b.running
		b.mu.Unlock()

		if !running || conn == nil {
			return
		}

		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		msg := new(dns.Msg)
		if err := msg.Unpack(buf[:n]); err == nil {
			for _, extra := range msg.Extra {
				if txt, ok := extra.(*dns.TXT); ok {
					isValid := false
					peerID := ""
					for _, t := range txt.Txt {
						if t == txtPrefix {
							isValid = true
						}
						if strings.HasPrefix(t, "id=") {
							peerID = strings.TrimPrefix(t, "id=")
						}
					}

					if isValid && peerID != "" && peerID != b.config.NodeID {
						portNum := b.config.Port // fallback
						for _, ex := range msg.Extra {
							if srv, ok := ex.(*dns.SRV); ok {
								portNum = int(srv.Port)
							}
						}
						b.validateAndRegister(ctx, peerID, src.String(), portNum)
					}
				}
			}
		}
	}
}

func (b *Beacon) validateAndRegister(ctx context.Context, peerID, addrStr string, portNum int) {
	authorized := false
	if len(b.config.AllowedPKIHashes) == 0 {
		authorized = true
	} else {
		for _, hash := range b.config.AllowedPKIHashes {
			if hash == peerID {
				authorized = true
				break
			}
		}
	}

	if !authorized {
		return
	}

	host, _, err := net.SplitHostPort(addrStr)
	if err != nil {
		host = addrStr
	}

	record := domain.PeerRecord{
		Identity: domain.NodeIdentity{ID: peerID},
		Address: domain.PeerAddress{
			Host: host,
			Port: portNum,
		},
		Capabilities: []string{"rpc"},
		LastSeen:     time.Now().UTC(),
	}

	_ = b.registry.Register(ctx, record)
}
