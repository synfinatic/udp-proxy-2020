package stages

import (
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

type mockWriter struct {
	data     []byte
	linkType layers.LinkType
	writeErr error
}

func (m *mockWriter) WritePacketData(data []byte) error {
	m.data = data
	return m.writeErr
}

func (m *mockWriter) LinkType() layers.LinkType {
	return m.linkType
}

func buildEthernetBusMessage(t *testing.T, srcIP, dstIP net.IP, payload []byte) proxy.BusMessage {
	t.Helper()

	ip := &layers.IPv4{
		SrcIP:    srcIP.To4(),
		DstIP:    dstIP.To4(),
		Protocol: layers.IPProtocolUDP,
		Version:  4,
		IHL:      5,
	}
	udp := &layers.UDP{SrcPort: 1234, DstPort: 5678}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatalf("SetNetworkLayerForChecksum failed: %v", err)
	}
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{1, 1, 1, 1, 1, 1},
		DstMAC:       net.HardwareAddr{2, 2, 2, 2, 2, 2},
		EthernetType: layers.EthernetTypeIPv4,
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(payload)); err != nil {
		t.Fatalf("SerializeLayers failed: %v", err)
	}

	return proxy.BusMessage{
		LinkType: layers.LinkTypeEthernet,
		Packet: &proxy.Packet{
			Raw: buf.Bytes(),
		},
	}
}

func TestTransmitterSink_Transmit(t *testing.T) {
	writer := &mockWriter{linkType: layers.LinkTypeEthernet}
	bus := make(chan proxy.BusMessage, 1)
	s := &TransmitterSink{
		Writer:           writer,
		Iname:            "eth0",
		HardwareAddr:     net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Broadcast:        true,
		BroadcastAddress: net.IP{2, 2, 2, 255}.To4(),
		PacketBus:        bus,
	}

	// Create a fake packet
	payload := []byte("test payload")
	msg := buildEthernetBusMessage(t, net.IP{1, 1, 1, 1}, net.IP{2, 2, 2, 2}, payload)

	s.transmit(msg)

	if writer.data == nil {
		t.Fatal("Expected data to be written to writer, but got nil")
	}

	// Verify the written data has the correct layers
	packet := gopacket.NewPacket(writer.data, layers.LayerTypeEthernet, gopacket.Default)
	if packet.Layer(layers.LayerTypeEthernet) == nil {
		t.Error("Expected Ethernet layer in transmitted packet")
	}
	if packet.Layer(layers.LayerTypeIPv4) == nil {
		t.Error("Expected IPv4 layer in transmitted packet")
	}
	if packet.Layer(layers.LayerTypeUDP) == nil {
		t.Error("Expected UDP layer in transmitted packet")
	}
}

func TestTransmitterSink_TransmitWithRegistry(t *testing.T) {
	reg, _ := NewRegistryProcessor(time.Hour, []string{"192.168.1.100"})
	writer := &mockWriter{linkType: layers.LinkTypeEthernet}
	bus := make(chan proxy.BusMessage, 1)
	s := &TransmitterSink{
		Writer:       writer,
		Iname:        "eth0",
		Registry:     reg,
		HardwareAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		PacketBus:    bus,
	}

	payload := []byte("test payload")
	msg := buildEthernetBusMessage(t, net.IP{1, 1, 1, 1}, net.IP{2, 2, 2, 2}, payload)

	s.transmit(msg)

	if writer.data == nil {
		t.Fatal("Expected data to be written to writer")
	}

	packet := gopacket.NewPacket(writer.data, layers.LayerTypeEthernet, gopacket.Default)
	ipLayer := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	if ipLayer.DstIP.String() != "192.168.1.100" {
		t.Errorf("Expected destination IP 192.168.1.100, got %s", ipLayer.DstIP)
	}
}
