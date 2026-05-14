package stages

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// RegistryProcessor handles client IP learning/caching.
type RegistryProcessor struct {
	mu      sync.RWMutex
	clients map[string]time.Time
	TTL     time.Duration
}

// NewRegistryProcessor creates a new RegistryProcessor.
// Returns an error if any of the fixed IPs are not valid IP addresses.
func NewRegistryProcessor(ttl time.Duration, fixedIPs []string) (*RegistryProcessor, error) {
	clients := make(map[string]time.Time)
	for _, ip := range fixedIPs {
		if net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("invalid fixed IP address: %q", ip)
		}
		clients[ip] = time.Time{} // zero value = immortal
	}
	return &RegistryProcessor{
		clients: clients,
		TTL:     ttl,
	}, nil
}

// GetClients returns a snapshot of all currently known client IPs.
func (r *RegistryProcessor) GetClients() []net.IP {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ips := make([]net.IP, 0, len(r.clients))
	for ipStr := range r.clients {
		ips = append(ips, net.ParseIP(ipStr))
	}
	return ips
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
	_, ok := r.clients[ip]
	return ok
}

func (r *RegistryProcessor) Process(pkt *proxy.Packet) (bool, error) {
	ipLayer := pkt.Packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return true, nil
	}
	ipv4 := ipLayer.(*layers.IPv4)
	srcIP := ipv4.SrcIP.String()

	r.mu.Lock()
	// only update if not a fixed client (zero time = immortal)
	if lastSeen, ok := r.clients[srcIP]; !ok || !lastSeen.IsZero() {
		r.clients[srcIP] = time.Now()
	}
	r.mu.Unlock()

	return true, nil
}

// Cleanup removes expired clients.
func (r *RegistryProcessor) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for ip, lastSeen := range r.clients {
		if lastSeen.IsZero() {
			continue // immortal fixed IP
		}
		if now.Sub(lastSeen) > r.TTL {
			delete(r.clients, ip)
		}
	}
}
