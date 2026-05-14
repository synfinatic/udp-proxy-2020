package stages

import (
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// TransmitterSink sends packets to a physical interface.
type TransmitterSink struct {
	Handle           *pcap.Handle
	Iname            string
	HardwareAddr     net.HardwareAddr
	Promisc          bool
	BroadcastAddress net.IP
	PacketBus        chan proxy.BusMessage
	Registry         *RegistryProcessor // Optional, for client list
}

func (s *TransmitterSink) Run() {
	for msg := range s.PacketBus {
		s.transmit(msg)
	}
}

func (s *TransmitterSink) transmit(msg proxy.BusMessage) {
	var eth layers.Ethernet
	var loop layers.Loopback
	var ip4 layers.IPv4
	var udp layers.UDP
	var payload gopacket.Payload

	if !s.decodePacket(msg, &eth, &loop, &ip4, &udp, &payload) {
		return
	}

	if !s.Promisc {
		// Send to broadcast address
		if err := s.sendToIP(msg, s.BroadcastAddress, eth, loop, ip4, udp, payload); err != nil {
			log.Warnf("Unable to send packet from %s out %s: %s", msg.Packet.ArrivalInterface, s.Iname, err)
		}
	} else if s.Registry != nil {
		// Send to all discovered clients
		clients := s.Registry.GetClients()
		if len(clients) == 0 {
			log.Debugf("%s: No clients discovered, dropping packet", s.Iname)
			return
		}
		for _, clientIP := range clients {
			if err := s.sendToIP(msg, clientIP, eth, loop, ip4, udp, payload); err != nil {
				log.Warnf("Unable to send packet from %s to %s out %s: %s", msg.Packet.ArrivalInterface, clientIP, s.Iname, err)
			}
		}
	}
}

func (s *TransmitterSink) decodePacket(msg proxy.BusMessage, eth *layers.Ethernet, loop *layers.Loopback, ip4 *layers.IPv4, udp *layers.UDP, payload *gopacket.Payload) bool {
	var parser *gopacket.DecodingLayerParser

	// If the source was Raw, we might not have an L2 header to decode
	// But our source sends the full packet data which includes whatever L2 was there.
	switch msg.LinkType {
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeLoopback, loop, ip4, udp, payload)
	case layers.LinkTypeEthernet:
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, eth, ip4, udp, payload)
	case layers.LinkTypeRaw:
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, ip4, udp, payload)
	default:
		log.Errorf("Unsupported link type: %v", msg.LinkType)
		return false
	}

	decoded := []gopacket.LayerType{}
	if err := parser.DecodeLayers(msg.Packet.Packet.Data(), &decoded); err != nil {
		log.Warnf("Unable to decode packet from %s: %v", msg.Packet.ArrivalInterface, err)
		return false
	}

	return true
}

func (s *TransmitterSink) sendToIP(msg proxy.BusMessage, dstIP net.IP, eth layers.Ethernet, loop layers.Loopback, ip4 layers.IPv4, udp layers.UDP, payload gopacket.Payload) error {
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	buffer := gopacket.NewSerializeBuffer()

	// Build layers from inside out
	// Payload
	if err := payload.SerializeTo(buffer, opts); err != nil {
		return fmt.Errorf("serialize payload: %w", err)
	}

	// UDP
	newUDP := layers.UDP{
		SrcPort: udp.SrcPort,
		DstPort: udp.DstPort,
	}
	// UDP checksum needs IP header for pseudo-checksum
	newIP4 := layers.IPv4{
		Version:  4,
		TTL:      ip4.TTL,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    ip4.SrcIP,
		DstIP:    dstIP,
	}
	if err := newUDP.SetNetworkLayerForChecksum(&newIP4); err != nil {
		return fmt.Errorf("set network layer for checksum: %w", err)
	}
	if err := newUDP.SerializeTo(buffer, opts); err != nil {
		return fmt.Errorf("serialize udp: %w", err)
	}

	// IP
	if err := newIP4.SerializeTo(buffer, opts); err != nil {
		return fmt.Errorf("serialize ip: %w", err)
	}

	// L2
	lt := s.Handle.LinkType()
	switch lt {
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		l := layers.Loopback{Family: layers.ProtocolFamilyIPv4}
		if err := l.SerializeTo(buffer, opts); err != nil {
			return fmt.Errorf("serialize loopback: %w", err)
		}
	case layers.LinkTypeEthernet:
		e := layers.Ethernet{
			DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			SrcMAC:       s.HardwareAddr,
			EthernetType: layers.EthernetTypeIPv4,
		}
		if err := e.SerializeTo(buffer, opts); err != nil {
			return fmt.Errorf("serialize ethernet: %w", err)
		}
	case layers.LinkTypeRaw:
		// No L2 header needed
	default:
		return fmt.Errorf("unsupported target link type: %v", lt)
	}

	return s.Handle.WritePacketData(buffer.Bytes())
}
