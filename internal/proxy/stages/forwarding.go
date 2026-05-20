package stages

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// PacketFeed is an interface for distributing packets to other interfaces.
type PacketFeed interface {
	Publish(msg proxy.BusMessage)
}

// ForwardingSink sends packets to a feed which distributes them to other pipelines.
type ForwardingSink struct {
	Feed     PacketFeed
	Iname    string
	LinkType layers.LinkType
}

func (s *ForwardingSink) Write(pkt *proxy.Packet) error {
	s.Feed.Publish(proxy.BusMessage{
		Packet:   pkt,
		LinkType: s.LinkType,
	})
	return nil
}

func (s *ForwardingSink) Close() error {
	return nil
}

func (s *ForwardingSink) Name() string {
	return "ForwardingSink(" + s.Iname + ")"
}
