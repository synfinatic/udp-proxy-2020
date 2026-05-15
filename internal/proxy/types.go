package proxy

import (
	"context"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Packet represents a packet flowing through the pipeline.
type Packet struct {
	// Raw is the original raw packet data.
	Raw []byte
	// Metadata contains capture information like timestamp and length.
	Metadata gopacket.CaptureInfo
	// Packet is the decoded gopacket representation.
	Packet gopacket.Packet
	// ArrivalInterface is the name of the interface where the packet was captured.
	ArrivalInterface string
}

// Source is an interface for reading packets from a network device or file.
type Source interface {
	Read(ctx context.Context) (*Packet, error)
	Close() error
}

// PacketWriter is an interface for writing packet data to a network device.
type PacketWriter interface {
	WritePacketData(data []byte) error
	LinkType() layers.LinkType
}

// Processor is an interface for filtering or transforming packets.
type Processor interface {
	// Process handles a packet. If it returns false, the packet is dropped
	// and no further processors or sinks are called.
	Process(pkt *Packet) (bool, error)
}

// Sink is an interface for writing packets to a network device, file, or log.
type Sink interface {
	Write(pkt *Packet) error
	Close() error
}
