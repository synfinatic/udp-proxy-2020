package stages

import (
	"bytes"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

func buildIPv4UDPPacket(t *testing.T, srcIP, dstIP net.IP, payload []byte) []byte {
	t.Helper()

	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    srcIP.To4(),
		DstIP:    dstIP.To4(),
	}
	udp := &layers.UDP{SrcPort: 1234, DstPort: 5678}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatalf("SetNetworkLayerForChecksum failed: %v", err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ip, udp, gopacket.Payload(payload)); err != nil {
		t.Fatalf("SerializeLayers failed: %v", err)
	}

	return buf.Bytes()
}

func TestTransformProcessor_Process_Success(t *testing.T) {
	originalRaw := buildIPv4UDPPacket(t, net.IP{10, 0, 0, 1}, net.IP{10, 0, 0, 2}, []byte("hello"))
	pkt := &proxy.Packet{
		Raw:    append([]byte(nil), originalRaw...),
		Packet: gopacket.NewPacket(originalRaw, layers.LayerTypeIPv4, gopacket.Default),
	}

	processor := &TransformProcessor{DestinationIP: net.IP{10, 0, 0, 99}}
	keep, err := processor.Process(pkt)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !keep {
		t.Fatal("expected packet to be kept")
	}
	if bytes.Equal(originalRaw, pkt.Raw) {
		t.Fatal("expected raw packet bytes to change after transform")
	}

	ipLayer := pkt.Packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		t.Fatal("expected IPv4 layer after transform")
	}
	ipv4 := ipLayer.(*layers.IPv4)
	if !ipv4.DstIP.Equal(net.IP{10, 0, 0, 99}) {
		t.Fatalf("expected destination IP 10.0.0.99, got %s", ipv4.DstIP)
	}

	if pkt.Packet.Layer(layers.LayerTypeUDP) == nil {
		t.Fatal("expected UDP layer after transform")
	}
}

func TestTransformProcessor_Process_DropsWithoutIPv4(t *testing.T) {
	pkt := &proxy.Packet{
		Raw:    []byte{0x01, 0x02, 0x03},
		Packet: gopacket.NewPacket([]byte{0x01, 0x02, 0x03}, layers.LayerTypeEthernet, gopacket.Default),
	}

	processor := &TransformProcessor{DestinationIP: net.IP{10, 0, 0, 99}}
	keep, err := processor.Process(pkt)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if keep {
		t.Fatal("expected packet to be dropped when IPv4 layer is missing")
	}
}

func TestTransformProcessor_Process_DropsWithoutUDP(t *testing.T) {
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.IP{10, 0, 0, 1}.To4(),
		DstIP:    net.IP{10, 0, 0, 2}.To4(),
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := ip.SerializeTo(buf, opts); err != nil {
		t.Fatalf("SerializeTo failed: %v", err)
	}

	raw := buf.Bytes()
	pkt := &proxy.Packet{
		Raw:    raw,
		Packet: gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default),
	}

	processor := &TransformProcessor{DestinationIP: net.IP{10, 0, 0, 99}}
	keep, err := processor.Process(pkt)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if keep {
		t.Fatal("expected packet to be dropped when UDP layer is missing")
	}
}
