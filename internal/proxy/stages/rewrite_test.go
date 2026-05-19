package stages

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

type captureSink struct {
	packets []*proxy.Packet
}

func (s *captureSink) Write(pkt *proxy.Packet) error {
	s.packets = append(s.packets, pkt)
	return nil
}

func (s *captureSink) Name() string { return "captureSink" }

func (s *captureSink) Close() error { return nil }

func buildUDPPacketForLinkType(t *testing.T, linkType layers.LinkType, srcIP, dstIP net.IP, srcMAC, dstMAC net.HardwareAddr, payload []byte, iname string) *proxy.Packet {
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

	var layersToSerialize []gopacket.SerializableLayer
	switch linkType {
	case layers.LinkTypeEthernet:
		layersToSerialize = append(layersToSerialize, &layers.Ethernet{SrcMAC: srcMAC, DstMAC: dstMAC, EthernetType: layers.EthernetTypeIPv4})
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		layersToSerialize = append(layersToSerialize, &layers.Loopback{Family: layers.ProtocolFamilyIPv4})
	case layers.LinkTypeRaw:
		// No L2 header.
	default:
		t.Fatalf("unsupported link type in test helper: %v", linkType)
	}

	layersToSerialize = append(layersToSerialize, ip, udp, gopacket.Payload(payload))
	if err := gopacket.SerializeLayers(buf, opts, layersToSerialize...); err != nil {
		t.Fatalf("SerializeLayers failed: %v", err)
	}

	raw := buf.Bytes()
	return &proxy.Packet{
		Raw: raw,
		Metadata: gopacket.CaptureInfo{
			Timestamp:     time.Now(),
			CaptureLength: len(raw),
			Length:        len(raw),
		},
		Packet:           packetFromLinkType(raw, linkType),
		ArrivalInterface: iname,
	}
}

func buildEthernetPacket(t *testing.T, srcIP, dstIP net.IP, srcMAC, dstMAC net.HardwareAddr, payload []byte, iname string) *proxy.Packet {
	return buildUDPPacketForLinkType(t, layers.LinkTypeEthernet, srcIP, dstIP, srcMAC, dstMAC, payload, iname)
}

func TestRewritePacketForEgress_CrossLinkType_EthernetToRaw(t *testing.T) {
	pkt := buildUDPPacketForLinkType(
		t,
		layers.LinkTypeEthernet,
		net.IP{10, 0, 0, 1},
		net.IP{10, 0, 0, 2},
		net.HardwareAddr{0, 1, 2, 3, 4, 5},
		net.HardwareAddr{6, 7, 8, 9, 10, 11},
		[]byte("hello"),
		"eth-in",
	)

	out, err := RewritePacketForEgress(pkt, RewriteOptions{
		TargetIP:       net.IP{10, 0, 1, 60},
		EgressLinkType: layers.LinkTypeRaw,
	})
	if err != nil {
		t.Fatalf("RewritePacketForEgress failed: %v", err)
	}

	if out.Packet.Layer(layers.LayerTypeEthernet) != nil {
		t.Fatal("did not expect ethernet layer on raw egress")
	}
	if out.Packet.Layer(layers.LayerTypeLoopback) != nil {
		t.Fatal("did not expect loopback layer on raw egress")
	}
	ipLayer := out.Packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		t.Fatal("expected ipv4 layer on raw egress")
	}
	ip4 := ipLayer.(*layers.IPv4)
	if !ip4.DstIP.Equal(net.IP{10, 0, 1, 60}) {
		t.Fatalf("unexpected dst ip: %s", ip4.DstIP)
	}
}

func TestRewritePacketForEgress_CrossLinkType_EthernetToLoopback(t *testing.T) {
	tests := []struct {
		name       string
		egressType layers.LinkType
	}{
		{name: "loop", egressType: layers.LinkTypeLoop},
		{name: "null", egressType: layers.LinkTypeNull},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := buildUDPPacketForLinkType(
				t,
				layers.LinkTypeEthernet,
				net.IP{10, 0, 0, 1},
				net.IP{10, 0, 0, 2},
				net.HardwareAddr{0, 1, 2, 3, 4, 5},
				net.HardwareAddr{6, 7, 8, 9, 10, 11},
				[]byte("hello"),
				"eth-in",
			)

			out, err := RewritePacketForEgress(pkt, RewriteOptions{
				TargetIP:       net.IP{10, 0, 1, 61},
				EgressLinkType: tc.egressType,
			})
			if err != nil {
				t.Fatalf("RewritePacketForEgress failed: %v", err)
			}

			if out.Packet.Layer(layers.LayerTypeEthernet) != nil {
				t.Fatal("did not expect ethernet layer on loop/null egress")
			}
			if out.Packet.Layer(layers.LayerTypeLoopback) == nil {
				t.Fatal("expected loopback layer on loop/null egress")
			}
		})
	}
}

func TestRewritePacketForEgress_CrossLinkType_RawToEthernet(t *testing.T) {
	pkt := buildUDPPacketForLinkType(
		t,
		layers.LinkTypeRaw,
		net.IP{10, 0, 0, 1},
		net.IP{10, 0, 0, 2},
		net.HardwareAddr{},
		net.HardwareAddr{},
		[]byte("hello"),
		"tun0",
	)

	out, err := RewritePacketForEgress(pkt, RewriteOptions{
		TargetIP:             net.IP{10, 0, 1, 62},
		TargetMAC:            net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		SourceMAC:            net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		EgressLinkType:       layers.LinkTypeEthernet,
		AllowBroadcastDstMAC: true,
	})
	if err != nil {
		t.Fatalf("RewritePacketForEgress failed: %v", err)
	}

	ethLayer := out.Packet.Layer(layers.LayerTypeEthernet)
	if ethLayer == nil {
		t.Fatal("expected ethernet layer")
	}
	eth := ethLayer.(*layers.Ethernet)
	if !bytes.Equal(eth.SrcMAC, net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}) {
		t.Fatalf("unexpected src mac: %s", eth.SrcMAC)
	}
	if !bytes.Equal(eth.DstMAC, net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}) {
		t.Fatalf("unexpected dst mac: %s", eth.DstMAC)
	}
}

func TestRewritePacketForEgress_UnicastEthernet(t *testing.T) {
	pkt := buildEthernetPacket(
		t,
		net.IP{10, 0, 0, 1},
		net.IP{10, 0, 0, 2},
		net.HardwareAddr{0, 1, 2, 3, 4, 5},
		net.HardwareAddr{6, 7, 8, 9, 10, 11},
		[]byte("hello"),
		"eth-in",
	)

	out, err := RewritePacketForEgress(pkt, RewriteOptions{
		TargetIP:               net.IP{10, 0, 1, 50},
		TargetMAC:              net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		SourceMAC:              net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		EgressLinkType:         layers.LinkTypeEthernet,
		AllowBroadcastDstMAC:   true,
		OutputArrivalInterface: "eth-out",
	})
	if err != nil {
		t.Fatalf("RewritePacketForEgress failed: %v", err)
	}

	if bytes.Equal(pkt.Raw, out.Raw) {
		t.Fatal("expected rewritten raw bytes to differ")
	}

	ethLayer := out.Packet.Layer(layers.LayerTypeEthernet)
	if ethLayer == nil {
		t.Fatal("expected ethernet layer")
	}
	eth := ethLayer.(*layers.Ethernet)
	if !bytes.Equal(eth.SrcMAC, net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}) {
		t.Fatalf("unexpected src mac: %s", eth.SrcMAC)
	}
	if !bytes.Equal(eth.DstMAC, net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}) {
		t.Fatalf("unexpected dst mac: %s", eth.DstMAC)
	}

	ipLayer := out.Packet.Layer(layers.LayerTypeIPv4)
	ip4 := ipLayer.(*layers.IPv4)
	if !ip4.DstIP.Equal(net.IP{10, 0, 1, 50}) {
		t.Fatalf("unexpected dst ip: %s", ip4.DstIP)
	}
	if out.ArrivalInterface != "eth-out" {
		t.Fatalf("expected rewritten arrival interface eth-out, got %s", out.ArrivalInterface)
	}
}

func TestRewritePacketForEgress_StaticUnknownMACFallsBackToBroadcast(t *testing.T) {
	pkt := buildEthernetPacket(
		t,
		net.IP{10, 0, 0, 1},
		net.IP{10, 0, 0, 2},
		net.HardwareAddr{0, 1, 2, 3, 4, 5},
		net.HardwareAddr{6, 7, 8, 9, 10, 11},
		[]byte("hello"),
		"eth-in",
	)

	out, err := RewritePacketForEgress(pkt, RewriteOptions{
		TargetIP:             net.IP{10, 0, 1, 51},
		SourceMAC:            net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		EgressLinkType:       layers.LinkTypeEthernet,
		AllowBroadcastDstMAC: true,
	})
	if err != nil {
		t.Fatalf("RewritePacketForEgress failed: %v", err)
	}

	eth := out.Packet.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
	if !bytes.Equal(eth.DstMAC, broadcastMAC) {
		t.Fatalf("expected broadcast destination MAC, got %s", eth.DstMAC)
	}
}

func TestRewritePacketForEgress_UnknownMACWithoutBroadcastFails(t *testing.T) {
	pkt := buildEthernetPacket(
		t,
		net.IP{10, 0, 0, 1},
		net.IP{10, 0, 0, 2},
		net.HardwareAddr{0, 1, 2, 3, 4, 5},
		net.HardwareAddr{6, 7, 8, 9, 10, 11},
		[]byte("hello"),
		"eth-in",
	)

	_, err := RewritePacketForEgress(pkt, RewriteOptions{
		TargetIP:             net.IP{10, 0, 1, 51},
		SourceMAC:            net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		EgressLinkType:       layers.LinkTypeEthernet,
		AllowBroadcastDstMAC: false,
	})
	if err == nil {
		t.Fatal("expected error when target mac missing on non-broadcast interface")
	}
}

func TestRouteSink_FanoutPerClient(t *testing.T) {
	reg, err := NewRegistryProcessorByInterface(time.Hour, map[string][]string{
		"eth-out": {"10.0.1.10", "10.0.1.11"},
	})
	if err != nil {
		t.Fatalf("registry init failed: %v", err)
	}

	capture := &captureSink{}
	writer := &mockWriter{linkType: layers.LinkTypeEthernet}
	route := &RouteSink{
		Iname:            "eth-out",
		Broadcast:        true,
		BroadcastAddress: net.IP{10, 0, 1, 255},
		HardwareAddr:     net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		Registry:         reg,
		LinkType:         writer,
		Sinks:            []proxy.Sink{capture},
	}

	pkt := buildEthernetPacket(
		t,
		net.IP{10, 0, 0, 1},
		net.IP{10, 0, 0, 2},
		net.HardwareAddr{0, 1, 2, 3, 4, 5},
		net.HardwareAddr{6, 7, 8, 9, 10, 11},
		[]byte("hello"),
		"eth-in",
	)

	if err := route.Write(pkt); err != nil {
		t.Fatalf("route write failed: %v", err)
	}

	if len(capture.packets) != 2 {
		t.Fatalf("expected 2 rewritten packets, got %d", len(capture.packets))
	}
}

func TestRouteSink_UsesEgressInterfaceClients(t *testing.T) {
	reg, err := NewRegistryProcessorByInterface(time.Hour, map[string][]string{
		"eth-in":  {"10.0.0.99"},
		"eth-out": {"10.0.1.42"},
	})
	if err != nil {
		t.Fatalf("registry init failed: %v", err)
	}

	capture := &captureSink{}
	writer := &mockWriter{linkType: layers.LinkTypeEthernet}
	route := &RouteSink{
		Iname:            "eth-out",
		Broadcast:        true,
		BroadcastAddress: net.IP{10, 0, 1, 255},
		HardwareAddr:     net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		Registry:         reg,
		LinkType:         writer,
		Sinks:            []proxy.Sink{capture},
	}

	pkt := buildEthernetPacket(
		t,
		net.IP{10, 0, 0, 1},
		net.IP{10, 0, 0, 2},
		net.HardwareAddr{0, 1, 2, 3, 4, 5},
		net.HardwareAddr{6, 7, 8, 9, 10, 11},
		[]byte("hello"),
		"eth-in",
	)

	if err := route.Write(pkt); err != nil {
		t.Fatalf("route write failed: %v", err)
	}

	if len(capture.packets) != 1 {
		t.Fatalf("expected 1 rewritten packet, got %d", len(capture.packets))
	}

	ipLayer := capture.packets[0].Packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		t.Fatal("missing IPv4 layer")
	}
	ipv4 := ipLayer.(*layers.IPv4)
	if !ipv4.DstIP.Equal(net.IP{10, 0, 1, 42}) {
		t.Fatalf("expected destination IP 10.0.1.42, got %s", ipv4.DstIP)
	}
}
