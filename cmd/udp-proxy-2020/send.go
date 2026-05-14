package main

import (
	"strings"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	log "github.com/sirupsen/logrus"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// Send is a struct for defining outgoing packets
type Send struct {
	packet   gopacket.Packet // packet data
	srcif    string          // interface it came in on
	linkType layers.LinkType // pcap LinkType of source interface
}

// SendPktFeed is a struct for collecting all channels to send packets
type SendPktFeed struct {
	lock    sync.Mutex           // lock
	senders map[string]chan Send // list of channels to send packets on
}

// Send is a function to send a packet out all the other interfaces other than srcif
func (s *SendPktFeed) Send(pkt *proxy.Packet, linkType layers.LinkType) {
	s.lock.Lock()
	for thisif, send := range s.senders {
		if strings.Compare(thisif, pkt.ArrivalInterface) == 0 {
			continue
		}
		log.Debugf("%s: sending out because we're not %s", thisif, pkt.ArrivalInterface)
		send <- Send{packet: pkt.Packet, srcif: pkt.ArrivalInterface, linkType: linkType}
	}
	s.lock.Unlock()
}

// RegisterSender registers a channel to receive packet data we want to send
func (s *SendPktFeed) RegisterSender(send chan Send, iname string) {
	s.lock.Lock()
	if s.senders == nil {
		s.senders = make(map[string]chan Send)
	}
	s.senders[iname] = send
	s.lock.Unlock()
}
