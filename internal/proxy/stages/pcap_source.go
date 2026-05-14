package stages

import (
	"io"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// PcapSource reads packets from a libpcap handle.
type PcapSource struct {
	handle       *pcap.Handle
	packetSource *gopacket.PacketSource
	packets      chan gopacket.Packet
	iname        string
}

// NewPcapSource creates a new PcapSource.
func NewPcapSource(handle *pcap.Handle, iname string) *PcapSource {
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	return &PcapSource{
		handle:       handle,
		packetSource: packetSource,
		packets:      packetSource.Packets(),
		iname:        iname,
	}
}

// Read reads the next packet from the PCAP handle.
func (s *PcapSource) Read() (*proxy.Packet, error) {
	select {
	case p, ok := <-s.packets:
		if !ok {
			return nil, io.EOF
		}

		return &proxy.Packet{
			Raw:              p.Data(),
			Metadata:         p.Metadata().CaptureInfo,
			Packet:           p,
			ArrivalInterface: s.iname,
		}, nil
	default:
		// Return nil, nil to allow context check in Pipeline.Run
		return nil, nil
	}
}

// Close closes the underlying pcap handle.
func (s *PcapSource) Close() error {
	if s.handle != nil {
		s.handle.Close()
	}
	return nil
}
