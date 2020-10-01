package main

import (
	"strings"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	log "github.com/sirupsen/logrus"
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
func (s *SendPktFeed) Send(p gopacket.Packet, srcif string, linkType layers.LinkType) {
	log.Debugf("Lock()???")
	s.lock.Lock()
	log.Debugf("Lock() achieved.  Sending out %d interfaces", len(s.senders)-1)
	for key, send := range s.senders {
		if strings.Compare(key, srcif) == 0 {
			continue
		}
		log.Debugf("%s: sending out because we're not %s", srcif, key)
		send <- Send{packet: p, srcif: srcif, linkType: linkType}
		log.Debugf("%s: sent", srcif)
	}
	s.lock.Unlock()
	log.Debugf("Unlock()")
}

// RegisterSender registers a channel to recieve packet data we want to send
func (s *SendPktFeed) RegisterSender(send chan Send, iname string) {
	s.lock.Lock()
	if s.senders == nil {
		s.senders = make(map[string]chan Send)
	}
	s.senders[iname] = send
	s.lock.Unlock()
}
