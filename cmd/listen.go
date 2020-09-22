package main

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
	"net"
	"strings"
	"sync"
	"time"
)

// Struct containing everything for an interface
type Listen struct {
	iface   string  // interface to use
	filter  string  // bpf filter string to listen on
	ports   []int32 // port(s) we listen for packets
	ipaddr  string  // dstip we send packets to
	promisc bool    // do we enable promisc on this interface?
	handle  *pcap.Handle
	raw     *ipv4.RawConn
	timeout time.Duration
	sendpkt chan Send // channel used to recieve packets we need to send
}

// takes the list of listen or promisc and returns a list of Listen
// which then can be initialized
func processListener(interfaces *[]string, lp []string, promisc bool, bpf_filter string, ports []int32, to time.Duration) []Listen {
	var ret = []Listen{}
	for _, i := range lp {
		s := strings.Split(i, "@")
		if len(s) != 2 {
			log.Fatalf("%s is invalid <interface>@<ipaddr>")
		}
		iface := s[0]
		ipaddr := s[1]

		iface_prefix := iface + "@"
		if stringPrefixInSlice(iface_prefix, *interfaces) {
			log.Fatalf("Can't specify the same interface (%s) multiple times", iface)
		}
		*interfaces = append(*interfaces, iface)
		new := Listen{
			iface:   iface,
			filter:  bpf_filter,
			ports:   ports,
			ipaddr:  ipaddr,
			timeout: to,
			promisc: promisc,
			handle:  nil,
			raw:     nil,
		}
		ret = append(ret, new)
	}
	return ret
}

// takes list of interfaces to listen on, if we should listen promiscuously,
// the BPF filter, list of ports and timeout and returns a list of processListener
func initalizeListeners(ifaces []string, promisc bool, bpf_filter string, ports []int32, timeout time.Duration) []Listen {
	// process our promisc and listen interfaces
	var interfaces = []string{}
	var listeners []Listen
	a := processListener(&interfaces, ifaces, promisc, bpf_filter, ports, timeout)
	for _, x := range a {
		listeners = append(listeners, x)
	}
	return listeners
}

// Does the heavy lifting of editing & sending the packet onwards
func (l *Listen) sendPacket(sndpkt Send) {
	h := ipv4.Header{
		Version:  4,
		Len:      4,
		TOS:      4,
		TotalLen: 4,
		ID:       0,
		FragOff:  0,
		TTL:      3,
		Protocol: 17,
		Checksum: 0,
		Src:      net.IP{},
		Dst:      net.IP{},
		Options:  []byte{},
	}

	// Need to tell golang what fields we want to control & the outbound interface
	err := l.raw.SetControlMessage(ipv4.FlagSrc|ipv4.FlagDst|ipv4.FlagInterface, true)
	if err != nil {
		log.Fatal(err)
	}

	var pktdata []byte
	var cm ipv4.ControlMessage
	if err := l.raw.WriteTo(&h, pktdata, &cm); err != nil {
		log.Errorf("Unable to send packet on %s: %s", l.iface, err)
	}

}

func (l *Listen) handlePackets(s *SendPktFeed, wg *sync.WaitGroup) {
	s.RegisterSender(l.sendpkt)
	packetSource := gopacket.NewPacketSource(l.handle, l.handle.LinkType())
	packets := packetSource.Packets()
	d, _ := time.ParseDuration("5s")
	ticker := time.Tick(d)
	for {
		select {
		case s := <-l.sendpkt:
			if l.iface == s.srcif {
				continue // don't send packets out the same interface them came in on
			}
			l.sendPacket(s)
			// send this packet out this interface
			// xportLayer := s.packet.TransportLayer()
			// appLayer := s.packet.ApplicationLayer()
		case packet := <-packets:
			// have a packet arriving on our interface
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil || packet.TransportLayer().LayerType() != layers.LayerTypeUDP {
				log.Warnf("%s: Unable packet", l.iface)
				continue
			} else if errx := packet.ErrorLayer(); errx != nil {
				log.Errorf("%s: Unable to decode: %s", l.iface, errx.Error())
			}
			s.Send(packet, l.iface)
		case <-ticker:
			log.Debugf("handlePackets(%s) ticker", l.iface)
		}
	}
}
