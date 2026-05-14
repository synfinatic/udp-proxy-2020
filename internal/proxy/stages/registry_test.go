package stages

import (
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
	ip := &layers.IPv4{SrcIP: []byte{192, 168, 1, 1}}

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
	ip *layers.IPv4
}

func (m *mockGopacket) Layer(l gopacket.LayerType) gopacket.Layer {
	if l == layers.LayerTypeIPv4 {
		return m.ip
	}
	return nil
}

func (m *mockGopacket) ErrorLayer() gopacket.ErrorLayer         { return nil }
func (m *mockGopacket) NetworkLayer() gopacket.NetworkLayer     { return m.ip }
func (m *mockGopacket) TransportLayer() gopacket.TransportLayer { return nil }
