package stages

import (
	"net"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// RegistryProcessor handles client IP learning/caching.
type RegistryProcessor struct {
	mu      sync.RWMutex
	Clients map[string]time.Time
	TTL     time.Duration
}

// NewRegistryProcessor creates a new RegistryProcessor.
func NewRegistryProcessor(ttl time.Duration, fixedIPs []string) *RegistryProcessor {
	clients := make(map[string]time.Time)
	for _, ip := range fixedIPs {
		clients[ip] = time.Time{} // use zero value for fixed/immortal clients
	}
	return &RegistryProcessor{
		Clients: clients,
		TTL:     ttl,
	}
}

// GetClients returns a list of all client IPs.
func (r *RegistryProcessor) GetClients() []net.IP {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ips []net.IP
	for ipStr := range r.Clients {
		ips = append(ips, net.ParseIP(ipStr))
	}
	return ips
}

func (r *RegistryProcessor) Process(pkt *proxy.Packet) (bool, error) {
	ipLayer := pkt.Packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return true, nil // Continue anyway, maybe another stage needs it
	}
	ipv4 := ipLayer.(*layers.IPv4)
	srcIP := ipv4.SrcIP.String()

	r.mu.Lock()
	// only update if not a fixed client (zero time)
	if lastSeen, ok := r.Clients[srcIP]; !ok || !lastSeen.IsZero() {
		r.Clients[srcIP] = time.Now()
	}
	r.mu.Unlock()

	return true, nil
}

// Cleanup removes expired clients.
func (r *RegistryProcessor) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for ip, lastSeen := range r.Clients {
		if lastSeen.IsZero() {
			continue // immortal fixed IP
		}
		if now.Sub(lastSeen) > r.TTL {
			delete(r.Clients, ip)
		}
	}
}
