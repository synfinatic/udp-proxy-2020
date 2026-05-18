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
	IP        net.IP
	Interface string
	LastSeen  time.Time // zero = immortal (fixed IP)
	MAC       net.HardwareAddr
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
		clients[keyForClient("", ipStr)] = ClientInfo{
			IP:        ip,
			Interface: "",
			LastSeen:  time.Time{},        // zero = immortal
			MAC:       net.HardwareAddr{}, // unknown
		}
	}
	return &RegistryProcessor{
		clients: clients,
		TTL:     ttl,
	}, nil
}

// NewRegistryProcessorByInterface creates a shared registry with fixed IPs keyed by interface.
func NewRegistryProcessorByInterface(ttl time.Duration, fixedIPs map[string][]string) (*RegistryProcessor, error) {
	clients := make(map[string]ClientInfo)
	for iname, ips := range fixedIPs {
		for _, ipStr := range ips {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				return nil, fmt.Errorf("invalid fixed IP address: %q", ipStr)
			}
			clients[keyForClient(iname, ipStr)] = ClientInfo{
				IP:        ip,
				Interface: iname,
				LastSeen:  time.Time{},        // zero = immortal
				MAC:       net.HardwareAddr{}, // unknown
			}
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
	for _, client := range r.clients {
		if client.IP.String() == ip {
			return true
		}
	}
	return false
}

// GetClientsForInterface returns currently known clients discovered on a specific interface.
func (r *RegistryProcessor) GetClientsForInterface(iname string) []ClientInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make([]ClientInfo, 0, len(r.clients))
	for _, info := range r.clients {
		if info.Interface == iname {
			clients = append(clients, info)
		}
	}
	return clients
}

func (r *RegistryProcessor) Process(pkt *proxy.Packet) (bool, error) {
	if pkt != nil {
		return r.ProcessForInterface(pkt.ArrivalInterface, pkt)
	}
	return r.ProcessForInterface("", nil)
}

// ProcessForInterface learns a packet source for the specified interface.
func (r *RegistryProcessor) ProcessForInterface(iname string, pkt *proxy.Packet) (bool, error) {
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
	if iname == "" {
		iname = pkt.ArrivalInterface
	}
	clientKey := keyForClient(iname, ipStr)

	r.mu.Lock()
	defer r.mu.Unlock()
	// only update if not a fixed client (zero time = immortal)
	if info, ok := r.clients[clientKey]; !ok || !info.LastSeen.IsZero() {
		r.clients[clientKey] = ClientInfo{
			IP:        srcIP,
			Interface: iname,
			MAC:       srcMAC,
			LastSeen:  time.Now(),
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

func (r *RegistryProcessor) Name() string {
	return "RegistryProcessor"
}

// RegistryLearnerProcessor binds a shared RegistryProcessor to one source interface.
type RegistryLearnerProcessor struct {
	Registry *RegistryProcessor
	Iname    string
}

func (p *RegistryLearnerProcessor) Process(pkt *proxy.Packet) (bool, error) {
	if p == nil || p.Registry == nil {
		return true, nil
	}
	return p.Registry.ProcessForInterface(p.Iname, pkt)
}

func (p *RegistryLearnerProcessor) Name() string {
	return fmt.Sprintf("RegistryLearnerProcessor:%s", p.Iname)
}

func keyForClient(iname, ip string) string {
	return iname + "|" + ip
}
