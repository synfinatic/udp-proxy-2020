package stages

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

type routeSinkTestSink struct {
	writes int
	pkts   []*proxy.Packet
	err    error
	closed int
}

func (s *routeSinkTestSink) Write(pkt *proxy.Packet) error {
	s.writes++
	s.pkts = append(s.pkts, pkt)
	return s.err
}

func (s *routeSinkTestSink) Name() string { return "routeSinkTestSink" }

func (s *routeSinkTestSink) Close() error {
	s.closed++
	return s.err
}

type routeSinkTestProcessor struct {
	calls int
	keep  bool
	err   error
}

func (p *routeSinkTestProcessor) Process(_ *proxy.Packet) (bool, error) {
	p.calls++
	if p.err != nil {
		return false, p.err
	}
	return p.keep, nil
}

func (p *routeSinkTestProcessor) Name() string { return "routeSinkTestProcessor" }

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
		Packet:           gopacket.NewPacket(raw, firstLayerTypeForLinkType(linkType), gopacket.Default),
		ArrivalInterface: iname,
	}
}

func firstLayerTypeForLinkType(linkType layers.LinkType) gopacket.LayerType {
	switch linkType {
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		return layers.LayerTypeLoopback
	case layers.LinkTypeEthernet:
		return layers.LayerTypeEthernet
	case layers.LinkTypeRaw:
		return layers.LayerTypeIPv4
	default:
		return gopacket.LayerTypePayload
	}
}

func buildEthernetPacket(t *testing.T, srcIP, dstIP net.IP, srcMAC, dstMAC net.HardwareAddr, payload []byte, iname string) *proxy.Packet {
	return buildUDPPacketForLinkType(t, layers.LinkTypeEthernet, srcIP, dstIP, srcMAC, dstMAC, payload, iname)
}

func TestRouteSink_Name(t *testing.T) {
	sink := &RouteSink{Iname: "eth9"}
	if got := sink.Name(); got != "RouteSink(eth9)" {
		t.Fatalf("unexpected name: %s", got)
	}
}

func TestRouteSink_TargetsForPacket_RegistryPreferredOverBroadcast(t *testing.T) {
	registry, err := NewRegistryProcessorByInterface(time.Hour, map[string][]string{
		"eth-out": {"10.0.0.10", "10.0.0.20"},
	})
	if err != nil {
		t.Fatalf("NewRegistryProcessorByInterface failed: %v", err)
	}

	sink := &RouteSink{
		Iname:            "eth-out",
		Broadcast:        true,
		BroadcastAddress: net.IP{10, 0, 0, 255},
		Registry:         registry,
	}

	targets := sink.targetsForPacket(&proxy.Packet{})
	if len(targets) != 2 {
		t.Fatalf("expected 2 registry targets, got %d", len(targets))
	}

	seen := map[string]bool{}
	for _, target := range targets {
		seen[target.IP.String()] = true
		if target.BroadcastDestMAC {
			t.Fatal("did not expect registry target to force broadcast MAC")
		}
		if !target.AllowBroadcastMAC {
			t.Fatal("expected registry target to allow broadcast fallback on broadcast interface")
		}
	}

	if !seen["10.0.0.10"] || !seen["10.0.0.20"] {
		t.Fatalf("missing expected registry targets: %+v", seen)
	}
}

func TestRouteSink_TargetsForPacket_BroadcastFallback(t *testing.T) {
	sink := &RouteSink{
		Iname:            "eth-out",
		Broadcast:        true,
		BroadcastAddress: net.IP{192, 168, 1, 255},
	}

	targets := sink.targetsForPacket(&proxy.Packet{})
	if len(targets) != 1 {
		t.Fatalf("expected 1 broadcast target, got %d", len(targets))
	}
	if !targets[0].IP.Equal(net.IP{192, 168, 1, 255}) {
		t.Fatalf("unexpected broadcast target IP: %s", targets[0].IP)
	}
	if !targets[0].BroadcastDestMAC {
		t.Fatal("expected broadcast target to force broadcast MAC")
	}
}

func TestRouteSink_Write_FansOutToTargetsAndSinks(t *testing.T) {
	registry, err := NewRegistryProcessorByInterface(time.Hour, map[string][]string{
		"eth-out": {"10.0.1.10", "10.0.1.20"},
	})
	if err != nil {
		t.Fatalf("NewRegistryProcessorByInterface failed: %v", err)
	}

	proc := &routeSinkTestProcessor{keep: true}
	sinkA := &routeSinkTestSink{}
	sinkB := &routeSinkTestSink{}
	routeSink := &RouteSink{
		Iname:            "eth-out",
		Broadcast:        true,
		BroadcastAddress: net.IP{10, 0, 1, 255},
		HardwareAddr:     net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		Registry:         registry,
		LinkType:         layers.LinkTypeEthernet,
		Processors:       []proxy.Processor{proc},
		Sinks:            []proxy.Sink{sinkA, sinkB},
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

	if err := routeSink.Write(pkt); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if proc.calls != 2 {
		t.Fatalf("expected processor to be called once per target (2), got %d", proc.calls)
	}
	if sinkA.writes != 2 || sinkB.writes != 2 {
		t.Fatalf("expected both sinks to receive packets for each target: sinkA=%d sinkB=%d", sinkA.writes, sinkB.writes)
	}

	targetIPs := map[string]bool{"10.0.1.10": false, "10.0.1.20": false}
	for _, rewritten := range sinkA.pkts {
		if rewritten.ArrivalInterface != "eth-out" {
			t.Fatalf("expected output arrival interface eth-out, got %s", rewritten.ArrivalInterface)
		}
		ipLayer := rewritten.Packet.Layer(layers.LayerTypeIPv4)
		if ipLayer == nil {
			t.Fatal("expected rewritten packet to contain IPv4 layer")
		}
		ip4 := ipLayer.(*layers.IPv4)
		targetIPs[ip4.DstIP.String()] = true
	}
	if !targetIPs["10.0.1.10"] || !targetIPs["10.0.1.20"] {
		t.Fatalf("expected both target IPs to be routed, got %+v", targetIPs)
	}
}

func TestRouteSink_Write_DropsWhenProcessorReturnsFalse(t *testing.T) {
	proc := &routeSinkTestProcessor{keep: false}
	capture := &routeSinkTestSink{}
	routeSink := &RouteSink{
		Iname:            "eth-out",
		Broadcast:        true,
		BroadcastAddress: net.IP{10, 0, 2, 255},
		HardwareAddr:     net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		LinkType:         layers.LinkTypeEthernet,
		Processors:       []proxy.Processor{proc},
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

	if err := routeSink.Write(pkt); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if proc.calls != 1 {
		t.Fatalf("expected processor to be called once, got %d", proc.calls)
	}
	if capture.writes != 0 {
		t.Fatalf("expected packet to be dropped before sinks, got %d writes", capture.writes)
	}
}

func TestRouteSink_Write_SkipsSinkWhenProcessorErrors(t *testing.T) {
	proc := &routeSinkTestProcessor{keep: true, err: errors.New("boom")}
	capture := &routeSinkTestSink{}
	routeSink := &RouteSink{
		Iname:            "eth-out",
		Broadcast:        true,
		BroadcastAddress: net.IP{10, 0, 3, 255},
		HardwareAddr:     net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		LinkType:         layers.LinkTypeEthernet,
		Processors:       []proxy.Processor{proc},
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

	if err := routeSink.Write(pkt); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if proc.calls != 1 {
		t.Fatalf("expected processor to be called once, got %d", proc.calls)
	}
	if capture.writes != 0 {
		t.Fatalf("expected packet to be dropped on processor error, got %d writes", capture.writes)
	}
}

func TestRouteSink_Write_RewriteErrorSkipsTarget(t *testing.T) {
	registry, err := NewRegistryProcessorByInterface(time.Hour, map[string][]string{
		"eth-out": {"10.0.4.10"},
	})
	if err != nil {
		t.Fatalf("NewRegistryProcessorByInterface failed: %v", err)
	}

	capture := &routeSinkTestSink{}
	routeSink := &RouteSink{
		Iname:        "eth-out",
		Broadcast:    false,
		HardwareAddr: net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		Registry:     registry,
		LinkType:     layers.LinkTypeEthernet,
		Sinks:        []proxy.Sink{capture},
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

	if err := routeSink.Write(pkt); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if capture.writes != 0 {
		t.Fatalf("expected rewrite failure to skip sink writes, got %d", capture.writes)
	}
}

func TestRouteSink_Close_ReturnsFirstErrorAndClosesAll(t *testing.T) {
	firstErr := errors.New("first close error")
	secondErr := errors.New("second close error")
	sinkA := &routeSinkTestSink{err: firstErr}
	sinkB := &routeSinkTestSink{err: secondErr}
	routeSink := &RouteSink{Sinks: []proxy.Sink{sinkA, sinkB}}

	err := routeSink.Close()
	if !errors.Is(err, firstErr) {
		t.Fatalf("expected first close error, got %v", err)
	}
	if sinkA.closed != 1 || sinkB.closed != 1 {
		t.Fatalf("expected all sinks to be closed once: sinkA=%d sinkB=%d", sinkA.closed, sinkB.closed)
	}
}

// Integration-style regression test: RouteSink should keep rewriting safely
// while TransmitterSink is in reconnect mode after write failures.
func TestRouteSink_WithTransmitterReconnect_DoesNotPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tx := &TransmitterSink{
		dm:     &proxy.DeviceManager{},
		Writer: &mockWriter{linkType: layers.LinkTypeEthernet, writeErr: errors.New("send: Device not configured")},
		Iname:  "wg0",
		ctx:    ctx,
		cancel: cancel,
	}

	route := &RouteSink{
		Iname:            "wg0",
		Broadcast:        true,
		BroadcastAddress: net.IP{10, 7, 0, 255},
		HardwareAddr:     net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		LinkType:         layers.LinkTypeEthernet,
		Sinks:            []proxy.Sink{tx},
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
		t.Fatalf("first route write failed: %v", err)
	}

	tx.mu.Lock()
	if !tx.reconnecting {
		tx.mu.Unlock()
		t.Fatal("expected transmitter to enter reconnecting state after write error")
	}
	if tx.Writer != nil {
		tx.mu.Unlock()
		t.Fatal("expected writer to be nil while reconnecting")
	}
	tx.mu.Unlock()

	// Second write should still succeed from RouteSink's perspective while
	// transmitter reconnect is in progress (packet is dropped gracefully).
	if err := route.Write(pkt); err != nil {
		t.Fatalf("second route write failed while reconnecting: %v", err)
	}
}
