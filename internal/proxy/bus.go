package proxy

import (
	"sync"

	"github.com/gopacket/gopacket/layers"
)

// BusMessage is a struct for defining outgoing packets across the bus.
type BusMessage struct {
	Packet   *Packet
	LinkType layers.LinkType
}

// PacketBus is a struct for collecting all channels to send packets.
type PacketBus struct {
	lock    sync.RWMutex
	senders map[string]chan BusMessage
}

// NewPacketBus creates a new PacketBus.
func NewPacketBus() *PacketBus {
	return &PacketBus{
		senders: make(map[string]chan BusMessage),
	}
}

// Publish sends a packet out all the other interfaces other than the source interface.
func (b *PacketBus) Publish(msg BusMessage) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	for iname, ch := range b.senders {
		if iname == msg.Packet.ArrivalInterface {
			continue
		}
		// Non-blocking send to avoid deadlocks if a buffer is full
		select {
		case ch <- msg:
		default:
			// In a real high-performance app, we should count drops here
		}
	}
}

// Subscribe registers a channel to receive packet data for a specific interface.
func (b *PacketBus) Subscribe(iname string, ch chan BusMessage) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.senders[iname] = ch
}

// Unsubscribe removes an interface from the bus.
func (b *PacketBus) Unsubscribe(iname string) {
	b.lock.Lock()
	defer b.lock.Unlock()
	delete(b.senders, iname)
}
