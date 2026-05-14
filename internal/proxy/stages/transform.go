package stages

import (
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// TransformProcessor handles packet editing (IP/UDP header modifications).
type TransformProcessor struct {
	// DestinationIP is the IP to set on outgoing packets.
	DestinationIP net.IP
}

func (t *TransformProcessor) Process(pkt *proxy.Packet) (bool, error) {
	// Re-calculating checksums and modifying the packet.
	packet := pkt.Packet

	ipLayer := packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return false, nil
	}
	ipv4 := ipLayer.(*layers.IPv4)

	udpLayer := packet.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		return false, nil
	}
	udp := udpLayer.(*layers.UDP)

	// Update destination IP
	ipv4.DstIP = t.DestinationIP

	// We need to re-serialize the packet to update checksums and the raw byte slice
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	buffer := gopacket.NewSerializeBuffer()

	// Set the network layer for the UDP checksum calculation
	if err := udp.SetNetworkLayerForChecksum(ipv4); err != nil {
		return false, fmt.Errorf("failed to set network layer for checksum: %w", err)
	}

	// Re-serialize the layers. Note: we are currently only handling IPv4/UDP.
	// We use the application layer (payload) and work outwards.
	payload := packet.ApplicationLayer()
	if payload == nil {
		return false, nil
	}

	err := gopacket.SerializeLayers(buffer, opts,
		ipv4,
		udp,
		gopacket.Payload(payload.Payload()),
	)
	if err != nil {
		return false, fmt.Errorf("failed to serialize layers: %w", err)
	}

	// Update the proxy.Packet with the new raw data
	pkt.Raw = buffer.Bytes()

	// Update the decoded packet as well so downstream processors see the change
	newPacket := gopacket.NewPacket(pkt.Raw, layers.LayerTypeIPv4, gopacket.Default)
	if newPacket.ErrorLayer() != nil {
		return false, fmt.Errorf("failed to re-decode modified packet: %w", newPacket.ErrorLayer().Error())
	}
	pkt.Packet = newPacket

	return true, nil
}
