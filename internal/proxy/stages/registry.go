package stages

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

type ClientInfo struct {
	IP       net.IP
	LastSeen time.Time // zero = immortal (fixed IP)
	MAC      net.HardwareAddr
}

// RegistryProcessor handles client IP learning/caching.
type RegistryProcessor struct {
	mu      sync.RWMutex
	clients map[string]ClientInfo
	TTL     time.Duration
}

// NewRegistryProcessor creates a new RegistryProcessor.
// Returns an error if any of the fixed IPs are not valid IP addresses.
func NewRegistryProcessor(ttl time.Duration, fixedIPs []string) (*RegistryProcessor, error) {
	clients := make(map[string]ClientInfo, len(fixedIPs))
	for _, ipStr := range fixedIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, fmt.Errorf("invalid fixed IP address: %q", ipStr)
		}
		clients[ipStr] = ClientInfo{
			IP:       ip,
			LastSeen: time.Time{},        // zero = immortal
			MAC:      net.HardwareAddr{}, // unknown
		}
	}
	return &RegistryProcessor{
		clients: clients,
		TTL:     ttl,
	}, nil
}

// GetClients returns a snapshot of all currently known client information.
func (r *RegistryProcessor) GetClients() []ClientInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make([]ClientInfo, 0, len(r.clients))
	for _, info := range r.clients {
		clients = append(clients, info)
	}
	return clients
}

// Len returns the number of currently tracked clients. Safe for concurrent use.
func (r *RegistryProcessor) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// Has reports whether the given IP string is in the registry. Safe for concurrent use.
func (r *RegistryProcessor) Has(ip string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for key := range r.clients {
		if key == ip {
			return true
		}
	}
	return false
}

func (r *RegistryProcessor) Process(pkt *proxy.Packet) (bool, error) {
	if r == nil || r.clients == nil {
		return true, nil
	}

	if pkt == nil || pkt.Packet == nil {
		return true, nil
	}

	ipLayer := pkt.Packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return true, nil
	}
	ipv4, ok := ipLayer.(*layers.IPv4)
	if !ok {
		return true, nil
	}
	srcIP := ipv4.SrcIP

	if len(srcIP) == 0 {
		return true, nil
	}

	var srcMAC net.HardwareAddr
	ethLayer := pkt.Packet.Layer(layers.LayerTypeEthernet)
	if ethLayer != nil {
		if eth, ok := ethLayer.(*layers.Ethernet); ok {
			srcMAC = eth.SrcMAC
		}
	}

	ipStr := srcIP.String()

	if ipStr == "" {
		return true, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// only update if not a fixed client (zero time = immortal)
	if info, ok := r.clients[ipStr]; !ok || !info.LastSeen.IsZero() {
		r.clients[ipStr] = ClientInfo{
			IP:       srcIP,
			MAC:      srcMAC,
			LastSeen: time.Now(),
		}
	}

	return true, nil
}

// Cleanup removes expired clients.
func (r *RegistryProcessor) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for ip, info := range r.clients {
		if info.LastSeen.IsZero() {
			continue // immortal fixed IP
		}
		if now.Sub(info.LastSeen) > r.TTL {
			delete(r.clients, ip)
		}
	}
}
