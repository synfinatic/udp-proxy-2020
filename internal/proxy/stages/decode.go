package stages

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

var decodeOutputMu sync.Mutex

// DecodeProcessor prints a one-line packet summary similar to tcpdump -e.
type DecodeProcessor struct {
	Iname     string
	Direction DecodeDirection
	Writer    io.Writer
}

type DecodeDirection string

const (
	DirectionInbound  DecodeDirection = "in"
	DirectionOutbound DecodeDirection = "out"
)

func NewDecodeProcessor(iname string, direction DecodeDirection, writer io.Writer) *DecodeProcessor {
	return &DecodeProcessor{
		Iname:     fmt.Sprintf("%s:%s", iname, direction),
		Direction: direction,
		Writer:    writer,
	}
}

func (d *DecodeProcessor) Process(pkt *proxy.Packet) (bool, error) {
	if pkt == nil || pkt.Packet == nil {
		return true, nil
	}

	if err := d.writePacket(pkt); err != nil {
		return false, err
	}

	return true, nil
}

func (d *DecodeProcessor) writePacket(pkt *proxy.Packet) error {
	if pkt == nil || pkt.Packet == nil {
		return nil
	}
	decode := d.formatPacket(pkt)
	writer := d.Writer
	if writer == nil {
		writer = os.Stdout
	}

	decodeOutputMu.Lock()
	defer decodeOutputMu.Unlock()

	if _, err := fmt.Fprintln(writer, decode); err != nil {
		return fmt.Errorf("write decode output: %w", err)
	}

	return nil
}

func (d *DecodeProcessor) Name() string {
	return fmt.Sprintf("DecodeProcessor:%s", d.Iname)
}

func (d *DecodeProcessor) formatPacket(pkt *proxy.Packet) string {
	parts := make([]string, 0, 4)

	if !pkt.Metadata.Timestamp.IsZero() {
		parts = append(parts, pkt.Metadata.Timestamp.Format("15:04:05.000000"))
	}

	if d.Iname != "" {
		parts = append(parts, d.Iname)
	}

	if linkSummary := formatLinkSummary(pkt.Packet); linkSummary != "" {
		parts = append(parts, linkSummary)
	}

	parts = append(parts, formatNetworkSummary(pkt.Packet))

	return strings.Join(parts, " ")
}

func formatLinkSummary(packet gopacket.Packet) string {
	var eth *layers.Ethernet
	var ok bool
	if packet == nil {
		return ""
	}

	if ethLayer := packet.Layer(layers.LayerTypeEthernet); ethLayer != nil {
		if eth, ok = ethLayer.(*layers.Ethernet); ok {
			return fmt.Sprintf(
				"%s > %s, ethertype %s, length %d:",
				formatMAC(eth.SrcMAC),
				formatMAC(eth.DstMAC),
				eth.EthernetType,
				len(packet.Data()),
			)
		}
	}

	if packet.Layer(layers.LayerTypeLoopback) != nil {
		if ipv4, ok := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4); ok {
			return fmt.Sprintf("loopback IPv4, IPID: 0x%04x, length %d:", ipv4.Id, len(packet.Data()))
		} else if ipv6, ok := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6); ok {
			return fmt.Sprintf("loopback IPv6, FlowLabel: %d, length %d:", ipv6.FlowLabel, len(packet.Data()))
		}
	}

	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if ipv4, ok := ipLayer.(*layers.IPv4); ok {
			return fmt.Sprintf("ethertype IPv4, IPID: 0x%04x, length %d:", ipv4.Id, len(packet.Data()))
		}
	}

	if ipLayer := packet.Layer(layers.LayerTypeIPv6); ipLayer != nil {
		if ipv6, ok := ipLayer.(*layers.IPv6); ok {
			return fmt.Sprintf("ethertype IPv6, FlowLabel: %d, length %d:", ipv6.FlowLabel, len(packet.Data()))
		}
	}

	if eth != nil {
		return fmt.Sprintf("ethertype (0x%04x), length %d:", uint16(eth.EthernetType), len(packet.Data()))
	}

	return fmt.Sprintf("unknown link, length %d:", len(packet.Data()))
}

func formatNetworkSummary(packet gopacket.Packet) string {
	if packet == nil {
		return "unknown"
	}

	ipLayer := packet.Layer(layers.LayerTypeIPv4)
	udpLayer := packet.Layer(layers.LayerTypeUDP)
	if ipLayer != nil && udpLayer != nil {
		ipv4, ipOK := ipLayer.(*layers.IPv4)
		udp, udpOK := udpLayer.(*layers.UDP)
		if ipOK && udpOK {
			payloadLen := 0
			if app := packet.ApplicationLayer(); app != nil {
				payloadLen = len(app.Payload())
			} else if udp.Length >= 8 {
				payloadLen = int(udp.Length - 8)
			}

			return fmt.Sprintf(
				"%s:%d > %s:%d: UDP, length %d, IPID: 0x%04x",
				ipv4.SrcIP,
				udp.SrcPort,
				ipv4.DstIP,
				udp.DstPort,
				payloadLen,
				ipv4.Id,
			)
		}
	}

	if ipLayer != nil {
		if ipv4, ok := ipLayer.(*layers.IPv4); ok {
			return fmt.Sprintf("%s > %s: %s", ipv4.SrcIP, ipv4.DstIP, ipv4.Protocol)
		}
	}

	return packet.Dump()
}

func formatMAC(mac net.HardwareAddr) string {
	if len(mac) == 0 {
		return "<unknown>"
	}
	return mac.String()
}
