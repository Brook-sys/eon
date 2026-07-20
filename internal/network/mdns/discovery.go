package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

var (
	ErrInvalidConfig   = errors.New("invalid mdns configuration")
	ErrDiscoveryFailed = errors.New("mdns discovery failed")
)

const (
	defaultMulticastAddress = "224.0.0.251:5353"
	maxPayloadSize          = 9000
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

	payload := []byte(fmt.Sprintf("NODE:%s:%d", b.config.NodeID, b.config.Port))

	b.sendAdvertisement(payload)

	for {
		select {
		case <-ctx.Done():
			b.Stop()
			return
		case <-ticker.C:
			b.sendAdvertisement(payload)
		}
	}
}

func (b *Beacon) sendAdvertisement(payload []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running || b.conn == nil {
		return
	}
	addr, _ := net.ResolveUDPAddr("udp4", b.config.MulticastAddress)
	_, _ = b.conn.WriteToUDP(payload, addr)
}

func (b *Beacon) listen(ctx context.Context) {
	buf := make([]byte, maxPayloadSize)

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

		msg := string(buf[:n])
		if strings.HasPrefix(msg, "NODE:") {
			parts := strings.Split(msg, ":")
			if len(parts) >= 2 {
				peerID := parts[1]
				portNum := b.config.Port
				if len(parts) >= 3 {
					p, err := strconv.Atoi(parts[2])
					if err == nil {
						portNum = p
					}
				}
				if peerID != b.config.NodeID {
					b.validateAndRegister(ctx, peerID, src.String(), portNum)
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
