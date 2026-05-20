package stages

import (
	"fmt"

	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// TransmitterSink sends packets to a physical interface.
type TransmitterSink struct {
	dm     *proxy.DeviceManager
	Writer proxy.PacketWriter
	Iname  string
}

// NewTransmitterSink creates a new TransmitterSink.
func NewTransmitterSink(dm *proxy.DeviceManager, iname string) (*TransmitterSink, error) {
	handle, err := dm.CreateWriterHandle(iname)
	if err != nil {
		return nil, err
	}

	return &TransmitterSink{
		dm:     dm,
		Writer: handle,
		Iname:  iname,
	}, nil
}

func (s *TransmitterSink) Name() string {
	return fmt.Sprintf("TransmitterSink(%s)", s.Iname)
}

func (s *TransmitterSink) Write(pkt *proxy.Packet) error {
	if pkt == nil {
		return nil
	}
	return s.Writer.WritePacketData(pkt.Raw)
}

// Close closes the underlying PCAP handle
func (s *TransmitterSink) Close() error {
	return s.dm.Close(s.Iname, proxy.Writer)
}
