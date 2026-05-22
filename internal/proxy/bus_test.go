package proxy

import (
	"testing"

	"github.com/gopacket/gopacket/layers"
)

func TestPacketBus(t *testing.T) {
	bus := NewPacketBus()
	ch1 := make(chan BusMessage, 1)
	ch2 := make(chan BusMessage, 1)

	bus.Subscribe("eth0", ch1)
	bus.Subscribe("eth1", ch2)

	pkt := &Packet{
		ArrivalInterface: "eth0",
	}
	msg := BusMessage{
		Packet:   pkt,
		LinkType: layers.LinkTypeEthernet,
	}

	bus.Publish(msg)

	// eth0 should NOT receive because it's the arrival interface
	select {
	case <-ch1:
		t.Error("eth0 received a packet it sent")
	default:
	}

	// eth1 SHOULD receive
	select {
	case received := <-ch2:
		if received.Packet.ArrivalInterface != "eth0" {
			t.Errorf("Expected arrival interface eth0, got %s", received.Packet.ArrivalInterface)
		}
	default:
		t.Error("eth1 did not receive the packet")
	}

	bus.Unsubscribe("eth1")
	bus.Publish(msg)

	select {
	case <-ch2:
		t.Error("eth1 received a packet after unsubscribing")
	default:
	}
}
