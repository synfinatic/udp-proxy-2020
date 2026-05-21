package stages

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

func TestPcapSourceRead_ReconnectsAfterInterfaceDownUp(t *testing.T) {
	oldPackets := make(chan gopacket.Packet)
	newPackets := make(chan gopacket.Packet, 1)

	var closeCalls int32
	var createCalls int32
	var pollCalls int32
	created := make(chan struct{}, 1)

	s := &PcapSource{
		iname:   "wg0",
		promisc: true,
		timeout: time.Second,
		packets: oldPackets,
		interfaceAvailable: func(string) bool {
			// First poll: down. Subsequent polls: up.
			return atomic.AddInt32(&pollCalls, 1) > 1
		},
		closeReaderHandle: func(string, proxy.PcapHandleDirection) error {
			atomic.AddInt32(&closeCalls, 1)
			return nil
		},
		createReaderHandle: func(string, bool, time.Duration) (*pcap.Handle, error) {
			atomic.AddInt32(&createCalls, 1)
			select {
			case created <- struct{}{}:
			default:
			}
			return nil, nil
		},
		newPacketSource: func(_ *pcap.Handle) (*gopacket.PacketSource, chan gopacket.Packet) {
			return nil, newPackets
		},
	}

	raw := []byte{0, 1, 2, 3, 4, 5}
	pkt := gopacket.NewPacket(raw, gopacket.LayerTypePayload, gopacket.Default)

	go func() {
		<-created
		newPackets <- pkt
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	out, err := s.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if out == nil {
		t.Fatal("expected packet, got nil")
	}
	if out.ArrivalInterface != "wg0" {
		t.Fatalf("expected arrival interface wg0, got %s", out.ArrivalInterface)
	}
	if atomic.LoadInt32(&closeCalls) == 0 {
		t.Fatal("expected reader handle close during reconnect")
	}
	if atomic.LoadInt32(&createCalls) == 0 {
		t.Fatal("expected reader handle create during reconnect")
	}
}
