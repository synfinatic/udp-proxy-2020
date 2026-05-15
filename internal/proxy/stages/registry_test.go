package stages

import (
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

func TestRegistryProcessor(t *testing.T) {
	reg, err := NewRegistryProcessor(100*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("NewRegistryProcessor failed: %v", err)
	}

	// Mock packet with IPv4 layer
	srcIP := net.IP{192, 168, 1, 1}
	ip := &layers.IPv4{SrcIP: srcIP}

	pkt := &proxy.Packet{
		Packet: &mockGopacket{ip: ip},
	}

	keep, err := reg.Process(pkt)
	if err != nil || !keep {
		t.Fatalf("Process failed: %v", err)
	}

	if !reg.Has("192.168.1.1") {
		t.Error("Expected IP 192.168.1.1 to be in registry")
	}

	// Test cleanup
	time.Sleep(150 * time.Millisecond)
	reg.Cleanup()

	if reg.Has("192.168.1.1") {
		t.Error("Expected IP 192.168.1.1 to be cleaned up")
	}
}

func TestNewRegistryProcessor_InvalidIP(t *testing.T) {
	_, err := NewRegistryProcessor(time.Minute, []string{"not-an-ip"})
	if err == nil {
		t.Error("Expected error for invalid fixed IP, got nil")
	}
}

type mockGopacket struct {
	gopacket.Packet
	ip  *layers.IPv4
	eth *layers.Ethernet
}

func (m *mockGopacket) Layer(l gopacket.LayerType) gopacket.Layer {
	if l == layers.LayerTypeIPv4 && m.ip != nil {
		return m.ip
	}
	if l == layers.LayerTypeEthernet && m.eth != nil {
		return m.eth
	}
	return nil
}

func (m *mockGopacket) ErrorLayer() gopacket.ErrorLayer         { return nil }
func (m *mockGopacket) NetworkLayer() gopacket.NetworkLayer     { return m.ip }
func (m *mockGopacket) TransportLayer() gopacket.TransportLayer { return nil }

func TestRegistryProcessor_FixedIPs(t *testing.T) {
	reg, err := NewRegistryProcessor(time.Hour, []string{"10.0.0.1"})
	if err != nil {
		t.Fatalf("NewRegistryProcessor failed: %v", err)
	}

	if !reg.Has("10.0.0.1") {
		t.Error("Expected fixed IP 10.0.0.1 to be present")
	}

	// Try to process a packet from the fixed IP, should not update LastSeen
	ip := &layers.IPv4{SrcIP: net.ParseIP("10.0.0.1")}
	pkt := &proxy.Packet{Packet: &mockGopacket{ip: ip}}
	_, _ = reg.Process(pkt)

	clients := reg.GetClients()
	for _, c := range clients {
		if c.IP.String() == "10.0.0.1" {
			if !c.LastSeen.IsZero() {
				t.Error("Fixed IP should have zero LastSeen (immortal)")
			}
		}
	}

	// Cleanup should not remove fixed IP
	reg.Cleanup()
	if !reg.Has("10.0.0.1") {
		t.Error("Fixed IP should not be removed by Cleanup")
	}
}

func TestRegistryProcessor_MACLearning(t *testing.T) {
	reg, _ := NewRegistryProcessor(time.Hour, nil)
	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	eth := &layers.Ethernet{SrcMAC: mac}
	ip := &layers.IPv4{SrcIP: net.ParseIP("192.168.1.10")}

	pkt := &proxy.Packet{
		Packet: &mockGopacket{ip: ip, eth: eth},
	}

	_, _ = reg.Process(pkt)

	clients := reg.GetClients()
	found := false
	for _, c := range clients {
		if c.IP.String() == "192.168.1.10" {
			found = true
			if c.MAC.String() != "00:11:22:33:44:55" {
				t.Errorf("Expected MAC 00:11:22:33:44:55, got %v", c.MAC)
			}
		}
	}
	if !found {
		t.Error("Client not found in registry")
	}
}

func TestRegistryProcessor_Len(t *testing.T) {
	reg, _ := NewRegistryProcessor(time.Hour, []string{"1.1.1.1", "2.2.2.2"})
	if reg.Len() != 2 {
		t.Errorf("Expected Len 2, got %d", reg.Len())
	}
}
