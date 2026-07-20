package mdns

import (
	"context"
	"net"
	"testing"
	"time"

	"motor-autonomo/internal/domain"

	"github.com/miekg/dns"
)

func TestBeacon_Integration_PeerDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	connA, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer connA.Close()

	connB, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer connB.Close()

	regA := &mockRegistry{peers: make(map[string]domain.PeerRecord)}
	cfgA := MDNSConfig{
		NodeID:           "node-a",
		AllowedPKIHashes: []string{"node-b"},
		Port:             8081,
	}
	beaconA := &Beacon{
		config:   cfgA,
		registry: regA,
		conn:     connA,
		running:  true,
	}

	regB := &mockRegistry{peers: make(map[string]domain.PeerRecord)}
	cfgB := MDNSConfig{
		NodeID:           "node-b",
		AllowedPKIHashes: []string{"node-a"},
		Port:             8082,
	}
	beaconB := &Beacon{
		config:   cfgB,
		registry: regB,
		conn:     connB,
		running:  true,
	}

	go beaconA.listen(ctx)
	go beaconB.listen(ctx)

	// Helper to generate DNS message
	buildMsg := func(nodeID string, port int) []byte {
		m := new(dns.Msg)
		m.Id = dns.Id()
		m.Response = true
		m.Authoritative = true

		srvName := nodeID + "." + mdnsService

		m.Answer = append(m.Answer, &dns.PTR{
			Hdr: dns.RR_Header{Name: mdnsService, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 120},
			Ptr: srvName,
		})

		m.Extra = append(m.Extra, &dns.SRV{
			Hdr:    dns.RR_Header{Name: srvName, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 120},
			Target: srvName,
			Port:   uint16(port),
		})
		m.Extra = append(m.Extra, &dns.TXT{
			Hdr: dns.RR_Header{Name: srvName, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 120},
			Txt: []string{
				txtPrefix,
				"id=" + nodeID,
			},
		})

		buf, _ := m.Pack()
		return buf
	}

	_, err = connA.WriteToUDP(buildMsg("node-a", 8081), connB.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("write A->B: %v", err)
	}

	_, err = connB.WriteToUDP(buildMsg("node-b", 8082), connA.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("write B->A: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	beaconA.mu.Lock()
	defer beaconA.mu.Unlock()
	if len(regA.peers) == 0 {
		t.Errorf("beacon A did not discover beacon B")
	} else if _, ok := regA.peers["node-b"]; !ok {
		t.Errorf("beacon A discovered wrong peer: %v", regA.peers)
	}

	beaconB.mu.Lock()
	defer beaconB.mu.Unlock()
	if len(regB.peers) == 0 {
		t.Errorf("beacon B did not discover beacon A")
	} else if _, ok := regB.peers["node-a"]; !ok {
		t.Errorf("beacon B discovered wrong peer: %v", regB.peers)
	}
}
