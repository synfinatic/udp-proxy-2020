package stages

import (
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

type mockPacketBus chan proxy.BusMessage

func (m mockPacketBus) Publish(msg proxy.BusMessage) {
	m <- msg
}

func TestForwardingSink(t *testing.T) {
	bus := make(mockPacketBus, 1)
	sink := &ForwardingSink{
		Feed:     bus,
		Iname:    "eth0",
		LinkType: layers.LinkTypeEthernet,
	}

	pkt := &proxy.Packet{
		ArrivalInterface: "eth0",
		Raw:              []byte("hello"),
	}

	err := sink.Write(pkt)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case msg := <-bus:
		if msg.Packet != pkt {
			t.Error("Message packet does not match original")
		}
		if msg.LinkType != layers.LinkTypeEthernet {
			t.Errorf("Expected link type Ethernet, got %v", msg.LinkType)
		}
	default:
		t.Error("No message published to bus")
	}
}

func TestRegistryProcessor_GetClients(t *testing.T) {
	registry, err := NewRegistryProcessor(0, []string{"192.168.1.1", "10.0.0.1"})
	if err != nil {
		t.Fatalf("NewRegistryProcessor failed: %v", err)
	}

	ips := registry.GetClients()
	if len(ips) != 2 {
		t.Errorf("Expected 2 clients, got %d", len(ips))
	}

	found1 := false
	found2 := false
	for _, ip := range ips {
		if ip.String() == "192.168.1.1" {
			found1 = true
		}
		if ip.String() == "10.0.0.1" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Errorf("Did not find both expected IPs: 192.168.1.1=%v, 10.0.0.1=%v", found1, found2)
	}
}

func TestFilterProcessor(t *testing.T) {
	filter := &FilterProcessor{Iname: "eth0"}

	// Valid UDP packet
	udpPacket := gopacket.NewPacket(
		[]byte{
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // dst mac
			0x00, 0x01, 0x02, 0x03, 0x04, 0x05, // src mac
			0x08, 0x00, // ether type ipv4
			0x45, 0x00, 0x00, 0x30, // v4, length 48
			0x00, 0x00, 0x40, 0x00, // no frag, ttl 64
			0x40, 0x11, 0x00, 0x00, // proto udp, checksum
			0x7f, 0x00, 0x00, 0x01, // src 127.0.0.1
			0x7f, 0x00, 0x00, 0x01, // dst 127.0.0.1
			0x04, 0xd2, 0x04, 0xd2, // src port 1234, dst port 1234
			0x00, 0x1c, 0x00, 0x00, // len 28, checksum
			0x01, 0x02, 0x03, 0x04, // payload
		},
		layers.LayerTypeEthernet,
		gopacket.Default,
	)

	pkt := &proxy.Packet{
		Packet:           udpPacket,
		ArrivalInterface: "eth0",
	}

	keep, err := filter.Process(pkt)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !keep {
		t.Error("Expected to keep valid UDP packet")
	}

	// Invalid packet (no network layer)
	invalidPacket := gopacket.NewPacket([]byte{1, 2, 3}, layers.LayerTypeEthernet, gopacket.Default)
	pkt.Packet = invalidPacket
	keep, _ = filter.Process(pkt)
	if keep {
		t.Error("Expected to drop invalid packet")
	}
}
