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
	p, ok := <-s.packets
	if !ok {
		return nil, io.EOF
	}

	return &proxy.Packet{
		Raw:              p.Data(),
		Metadata:         p.Metadata().CaptureInfo,
		Packet:           p,
		ArrivalInterface: s.iname,
	}, nil
}

// Close is a no-op as the handle might be shared, but satisfies the interface.
func (s *PcapSource) Close() error {
	return nil
}
