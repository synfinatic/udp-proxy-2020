package stages

import (
	"os"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// PcapFileSink writes packets to a PCAP file.
type PcapFileSink struct {
	Writer *pcapgo.Writer
	File   *os.File
}

func (s *PcapFileSink) Write(pkt *proxy.Packet) error {
	if s.Writer == nil {
		return nil
	}
	return s.Writer.WritePacket(pkt.Metadata, pkt.Raw)
}

func (s *PcapFileSink) Close() error {
	if s.File != nil {
		return s.File.Close()
	}
	return nil
}
