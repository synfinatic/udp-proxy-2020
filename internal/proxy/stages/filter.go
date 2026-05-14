package stages

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// FilterProcessor drops packets that are not valid UDP/IPv4.
type FilterProcessor struct {
	Iname string
}

func (f *FilterProcessor) Process(pkt *proxy.Packet) (bool, error) {
	if pkt.Packet.NetworkLayer() == nil ||
		pkt.Packet.TransportLayer() == nil ||
		pkt.Packet.TransportLayer().LayerType() != layers.LayerTypeUDP {
		return false, nil
	}

	if pkt.Packet.ErrorLayer() != nil {
		return false, nil
	}

	return true, nil
}
