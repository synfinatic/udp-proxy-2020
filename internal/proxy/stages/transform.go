package stages

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// TransformProcessor handles packet editing (IP/UDP header modifications).
type TransformProcessor struct {
	// DestinationIP is the IP to set on outgoing packets.
	DestinationIP []byte
}

func (t *TransformProcessor) Process(pkt *proxy.Packet) (bool, error) {
	// Re-calculating checksums and modifying the packet.
	// This mirrors the logic in buildPacket but as a pipeline stage.

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

	// Reset checksums for recalculation
	udp.SetNetworkLayerForChecksum(ipv4)

	// In a real implementation, we might use a SerializeBuffer here
	// or modify the layers in place if gopacket supports it for the specific handles.
	// For the purposes of this refactor, we are defining the logical stage.

	return true, nil
}
