package stages

import (
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// PcapFileSink writes packets to a PCAP file.
type PcapFileSink struct {
	Writer *pcapgo.Writer
}

func (s *PcapFileSink) Write(pkt *proxy.Packet) error {
	if s.Writer == nil {
		return nil
	}
	// Note: We use the captured metadata
	return s.Writer.WritePacket(pkt.Metadata, pkt.Raw)
}

func (s *PcapFileSink) Close() error {
	return nil
}
