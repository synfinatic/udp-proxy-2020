package stages

import (
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

type mockWriter struct {
	data     []byte
	linkType layers.LinkType
	writeErr error
}

func (m *mockWriter) WritePacketData(data []byte) error {
	m.data = data
	return m.writeErr
}

func (m *mockWriter) LinkType() layers.LinkType {
	return m.linkType
}

func TestTransmitterSink_Write(t *testing.T) {
	writer := &mockWriter{linkType: layers.LinkTypeEthernet}
	s := &TransmitterSink{
		Writer: writer,
		Iname:  "eth0",
	}

	pkt := &proxy.Packet{Raw: []byte("payload")}
	if err := s.Write(pkt); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if writer.data == nil {
		t.Fatal("Expected data to be written to writer, but got nil")
	}
}

func TestTransmitterSink_WriteNilPacket(t *testing.T) {
	writer := &mockWriter{linkType: layers.LinkTypeEthernet}
	s := &TransmitterSink{
		Writer: writer,
		Iname:  "eth0",
	}

	if err := s.Write(nil); err != nil {
		t.Fatalf("Write(nil) failed: %v", err)
	}

	if writer.data == nil {
		return
	}
}
