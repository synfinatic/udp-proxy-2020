package stages

import (
	"context"
	"io"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// PcapSource reads packets from a libpcap handle.
type PcapSource struct {
	dm           *proxy.DeviceManager
	handle       *pcap.Handle
	packetSource *gopacket.PacketSource
	packets      chan gopacket.Packet
	iname        string
}

// NewPcapSource creates a new PcapSource.
func NewPcapSource(dm *proxy.DeviceManager, iname string, promisc bool, timeout time.Duration) (*PcapSource, error) {
	handle, err := dm.CreateReaderHandle(iname, promisc, timeout)
	if err != nil {
		return nil, err
	}
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	return &PcapSource{
		dm:           dm,
		handle:       handle,
		packetSource: packetSource,
		packets:      packetSource.Packets(),
		iname:        iname,
	}, nil
}

func (s *PcapSource) Handle() *pcap.Handle {
	return s.handle
}

// Read reads the next packet from the PCAP handle.
func (s *PcapSource) Read(ctx context.Context) (*proxy.Packet, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
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
	}
}

// Close closes the underlying PCAP handle
func (s *PcapSource) Close() error {
	return s.dm.Close(s.iname, proxy.Reader)
}
