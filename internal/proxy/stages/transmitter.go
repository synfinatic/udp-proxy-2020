package stages

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// TransmitterSink sends packets to a physical interface.
type TransmitterSink struct {
	dm               *proxy.DeviceManager
	Handle           *pcap.Handle
	Iname            string
	HardwareAddr     net.HardwareAddr
	Broadcast        bool
	BroadcastAddress net.IP
	PacketBus        chan proxy.BusMessage
	Registry         *RegistryProcessor // Optional, for client list
}

// NewTransmitterSink creates a new TransmitterSink.
func NewTransmitterSink(dm *proxy.DeviceManager,
	iname string,
	broadcast bool,
	broadcastAddress net.IP,
	hardwareAddr net.HardwareAddr,
	busChan chan proxy.BusMessage,
	registry *RegistryProcessor) (*TransmitterSink, error) {
	handle, err := dm.CreateWriterHandle(iname)
	if err != nil {
		return nil, err
	}
	return &TransmitterSink{
		dm:               dm,
		Handle:           handle,
		Iname:            iname,
		Broadcast:        broadcast,
		BroadcastAddress: broadcastAddress,
		HardwareAddr:     hardwareAddr,
		PacketBus:        busChan,
		Registry:         registry,
	}, nil
}

// Run starts the transmitter loop which listens for packets on the PacketBus and transmits them.
// To stop the loop, cancel the provided context.
func (s *TransmitterSink) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.PacketBus:
			s.transmit(msg)
		}
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

	if s.Registry != nil {
		discoveredClients := false

		// Send to all discovered clients
		clients := s.Registry.GetClients()
		if len(clients) == 0 {
			slog.Debug("No clients discovered, dropping packet", "interface", s.Iname)
			return
		}
		for _, client := range clients {
			discoveredClients = true
			if err := s.sendToIP(msg, client.IP, eth, loop, ip4, udp, payload); err != nil {
				slog.Warn("Unable to send packet to client", "from", msg.Packet.ArrivalInterface, "client", client.IP, "to_interface", s.Iname, "error", err)
			}
		}
		if discoveredClients {
			return
		}
	}

	// If broadcast is enabled on interface, and there are no discovered clients, send to the broadcast address
	if s.Broadcast {
		if err := s.sendToIP(msg, s.BroadcastAddress, eth, loop, ip4, udp, payload); err != nil {
			slog.Warn("Unable to send packet", "from", msg.Packet.ArrivalInterface, "to_interface", s.Iname, "error", err)
		}
		return
	}

	slog.Warn("Unable to send packet, dropping packet", "interface", s.Iname)
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
		slog.Error("Unsupported link type", "link_type", msg.LinkType)
		return false
	}

	decoded := []gopacket.LayerType{}
	if err := parser.DecodeLayers(msg.Packet.Raw, &decoded); err != nil {
		slog.Warn("Unable to decode packet", "from", msg.Packet.ArrivalInterface, "error", err)
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

	// Build layers from top down.
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

// Close closes the underlying PCAP handle
func (s *TransmitterSink) Close() error {
	return s.dm.Close(s.Iname, proxy.Writer)
}
