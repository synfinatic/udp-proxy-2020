package rewrite

import (
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

var broadcastMAC = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// Options configures L2/L3 modifications for outbound packets.
type Options struct {
	TargetIP               net.IP
	TargetMAC              net.HardwareAddr
	SourceMAC              net.HardwareAddr
	EgressLinkType         layers.LinkType
	AllowBroadcastDstMAC   bool
	ForceBroadcastDestMAC  bool
	ArrivalInterface       string
	OutputArrivalInterface string
}

// PacketForEgress applies destination/source L2/L3 changes and returns a new packet.
func PacketForEgress(pkt *proxy.Packet, opts Options) (*proxy.Packet, error) {
	if pkt == nil || pkt.Packet == nil {
		return nil, fmt.Errorf("packet is nil")
	}

	ipLayer := pkt.Packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return nil, fmt.Errorf("packet missing IPv4 layer")
	}
	ipv4, ok := ipLayer.(*layers.IPv4)
	if !ok {
		return nil, fmt.Errorf("packet IPv4 layer decode error")
	}

	udpLayer := pkt.Packet.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		return nil, fmt.Errorf("packet missing UDP layer")
	}
	udp, ok := udpLayer.(*layers.UDP)
	if !ok {
		return nil, fmt.Errorf("packet UDP layer decode error")
	}

	payload := udp.Payload
	if len(payload) == 0 {
		if app := pkt.Packet.ApplicationLayer(); app != nil {
			payload = app.Payload()
		}
	}

	newIP4 := layers.IPv4{
		Version:    4,
		IHL:        5,
		TTL:        ipv4.TTL,
		Protocol:   layers.IPProtocolUDP,
		SrcIP:      ipv4.SrcIP.To4(),
		DstIP:      opts.TargetIP.To4(),
		Id:         ipv4.Id,
		Flags:      ipv4.Flags,
		FragOffset: ipv4.FragOffset,
	}

	newUDP := layers.UDP{
		SrcPort: udp.SrcPort,
		DstPort: udp.DstPort,
	}
	if err := newUDP.SetNetworkLayerForChecksum(&newIP4); err != nil {
		return nil, fmt.Errorf("set network layer for checksum: %w", err)
	}

	sz := gopacket.NewSerializeBuffer()
	serializeOpts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}

	var layersToSerialize []gopacket.SerializableLayer
	switch opts.EgressLinkType {
	case layers.LinkTypeEthernet:
		dstMAC := opts.TargetMAC
		if opts.ForceBroadcastDestMAC {
			dstMAC = broadcastMAC
		} else if len(dstMAC) == 0 {
			if !opts.AllowBroadcastDstMAC {
				return nil, fmt.Errorf("missing destination MAC for non-broadcast interface")
			}
			dstMAC = broadcastMAC
		}
		if len(opts.SourceMAC) == 0 {
			return nil, fmt.Errorf("missing source MAC for ethernet egress")
		}
		eth := &layers.Ethernet{
			DstMAC:       dstMAC,
			SrcMAC:       opts.SourceMAC,
			EthernetType: layers.EthernetTypeIPv4,
		}
		layersToSerialize = append(layersToSerialize, eth)
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		layersToSerialize = append(layersToSerialize, &layers.Loopback{Family: layers.ProtocolFamilyIPv4})
	case layers.LinkTypeRaw, proxy.LinkTypeRawOpenBSD, proxy.LinkTypeRawOthers:
		// No L2 header.
	default:
		return nil, fmt.Errorf("unsupported egress link type: %v", opts.EgressLinkType)
	}

	layersToSerialize = append(layersToSerialize, &newIP4, &newUDP, gopacket.Payload(payload))

	if err := gopacket.SerializeLayers(sz, serializeOpts, layersToSerialize...); err != nil {
		return nil, fmt.Errorf("serialize rewritten packet: %w", err)
	}

	raw := sz.Bytes()
	arrivalIf := pkt.ArrivalInterface
	if opts.OutputArrivalInterface != "" {
		arrivalIf = opts.OutputArrivalInterface
	}

	return &proxy.Packet{
		Metadata:         pkt.Metadata,
		Raw:              raw,
		Packet:           packetFromLinkType(raw, opts.EgressLinkType),
		ArrivalInterface: arrivalIf,
	}, nil
}

func packetFromLinkType(raw []byte, linkType layers.LinkType) gopacket.Packet {
	switch linkType {
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		return gopacket.NewPacket(raw, layers.LayerTypeLoopback, gopacket.Default)
	case layers.LinkTypeEthernet:
		return gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default)
	case layers.LinkTypeRaw, proxy.LinkTypeRawOpenBSD, proxy.LinkTypeRawOthers:
		return gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)
	default:
		return gopacket.NewPacket(raw, gopacket.LayerTypePayload, gopacket.Default)
	}
}
