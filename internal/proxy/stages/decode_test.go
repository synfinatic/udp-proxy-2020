package stages

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

func TestDecodeProcessor_Process_WritesTcpdumpStyleSummary(t *testing.T) {
	srcMAC, err := net.ParseMAC("00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("ParseMAC src failed: %v", err)
	}
	dstMAC, err := net.ParseMAC("66:77:88:99:aa:bb")
	if err != nil {
		t.Fatalf("ParseMAC dst failed: %v", err)
	}

	eth := &layers.Ethernet{
		SrcMAC:       srcMAC,
		DstMAC:       dstMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.IP{192, 168, 1, 10}.To4(),
		DstIP:    net.IP{239, 255, 255, 250}.To4(),
	}
	udp := &layers.UDP{SrcPort: 5678, DstPort: 1900}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatalf("SetNetworkLayerForChecksum failed: %v", err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	payload := gopacket.Payload([]byte("hello"))
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, payload); err != nil {
		t.Fatalf("SerializeLayers failed: %v", err)
	}

	raw := buf.Bytes()
	pkt := &proxy.Packet{
		Raw:      raw,
		Packet:   gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default),
		Metadata: gopacket.CaptureInfo{Timestamp: time.Date(2026, time.May, 18, 12, 34, 56, 123000000, time.UTC)},
	}

	var out bytes.Buffer
	processor := &DecodeProcessor{Iname: "eth0", Writer: &out}

	keep, err := processor.Process(pkt)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !keep {
		t.Fatal("expected packet to be kept")
	}

	got := out.String()
	wants := []string{
		"12:34:56.123000",
		"eth0",
		"00:11:22:33:44:55 > 66:77:88:99:aa:bb, ethertype IPv4 (0x0800)",
		"192.168.1.10.5678 > 239.255.255.250.1900: UDP, length 5",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected output to end with newline, got %q", got)
	}
}

func TestDecodeProcessor_Process_NilPacketNoOutput(t *testing.T) {
	var out bytes.Buffer
	processor := &DecodeProcessor{Writer: &out}

	keep, err := processor.Process(nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !keep {
		t.Fatal("expected packet to be kept")
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output, got %q", out.String())
	}
}
