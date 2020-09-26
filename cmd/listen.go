package main

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"net"
	"strings"
	"sync"
	"time"
)

const SendBufferSize = 100

// Struct containing everything for an interface
type Listen struct {
	iname   string         // interface to use
	iface   *net.Interface // interface descriptor
	filter  string         // bpf filter string to listen on
	ports   []int32        // port(s) we listen for packets
	ipaddr  string         // dstip we send packets to
	promisc bool           // do we enable promisc on this interface?
	handle  *pcap.Handle
	timeout time.Duration
	sendpkt chan Send // channel used to recieve packets we need to send
}

// List of LayerTypes we support in sendPacket()
var validLinkTypes = []layers.LinkType{
	layers.LinkTypeLoop,
	layers.LinkTypeEthernet,
	layers.LinkTypeNull,
}

// takes the list of listen or promisc and returns a list of Listen
// which then can be initialized
func processListener(interfaces *[]string, lp []string, promisc bool, bpf_filter string, ports []int32, to time.Duration) []Listen {
	var ret = []Listen{}
	for _, i := range lp {
		s := strings.Split(i, "@")
		if len(s) != 2 {
			log.Fatalf("%s is invalid.  Expected: <interface>@<ipaddr>", i)
		}
		iname := s[0]
		ipaddr := s[1]

		iname_prefix := iname + "@"
		if stringPrefixInSlice(iname_prefix, *interfaces) {
			log.Fatalf("Can't specify the same interface (%s) multiple times", iname)
		}
		*interfaces = append(*interfaces, iname)

		netif, err := net.InterfaceByName(iname)
		if err != nil {
			log.Fatalf("Unable to get network index for %s: %s", iname, err)
		}
		log.Debugf("%s: ifIndex: %d", iname, netif.Index)

		new := Listen{
			iname:   iname,
			iface:   netif,
			filter:  bpf_filter,
			ports:   ports,
			ipaddr:  ipaddr,
			timeout: to,
			promisc: promisc,
			handle:  nil,
			sendpkt: make(chan Send, SendBufferSize),
		}
		ret = append(ret, new)
	}
	return ret
}

// takes list of interfaces to listen on, if we should listen promiscuously,
// the BPF filter, list of ports and timeout and returns a list of processListener
func initalizeListeners(inames []string, promisc bool, bpf_filter string, ports []int32, timeout time.Duration) []Listen {
	// process our promisc and listen interfaces
	var interfaces = []string{}
	var listeners []Listen
	a := processListener(&interfaces, inames, promisc, bpf_filter, ports, timeout)
	for _, x := range a {
		listeners = append(listeners, x)
	}
	return listeners
}

// Our goroutine for processing packets
func (l *Listen) handlePackets(s *SendPktFeed, wg *sync.WaitGroup) {
	// add ourself as a sender
	s.RegisterSender(l.sendpkt, l.iname)

	// get packets from libpcap
	packetSource := gopacket.NewPacketSource(l.handle, l.handle.LinkType())
	packets := packetSource.Packets()

	// This timer is nice for debugging
	d, _ := time.ParseDuration("5s")
	ticker := time.Tick(d)

	// loop forever and ever and ever
	for {
		select {
		case s := <-l.sendpkt: // packet arrived from another interface
			l.sendPacket(s)
		case packet := <-packets: // packet arrived on this interfaces
			// is it legit?
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil || packet.TransportLayer().LayerType() != layers.LayerTypeUDP {
				log.Warnf("%s: Invalid packet", l.iname)
				continue
			} else if errx := packet.ErrorLayer(); errx != nil {
				log.Errorf("%s: Unable to decode: %s", l.iname, errx.Error())
			}

			log.Debugf("%s: received packet and fowarding onto other interfaces", l.iname)
			s.Send(packet, l.iname, l.handle.LinkType())
		case <-ticker: // our timer
			log.Debugf("handlePackets(%s) ticker", l.iname)
		}
	}
}

// Does the heavy lifting of editing & sending the packet onwards
func (l *Listen) sendPacket(sndpkt Send) {
	var eth layers.Ethernet
	var loop layers.Loopback // BSD NULL/Loopback used for OpenVPN tunnels/etc
	var ip4 layers.IPv4      // we only support v4
	var udp layers.UDP
	var payload gopacket.Payload
	var parser *gopacket.DecodingLayerParser

	log.Debugf("processing packet from %s on %s", sndpkt.srcif, l.iname)

	switch sndpkt.linkType {
	case layers.LinkTypeNull:
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeLoopback, &loop, &ip4, &udp, &payload)
	case layers.LinkTypeLoop:
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeLoopback, &loop, &ip4, &udp, &payload)
	case layers.LinkTypeEthernet:
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &udp, &payload)
	default:
		log.Fatalf("Unsupported source linktype: 0x%02x", sndpkt.linkType)
		return
	}

	// try decoding our packet
	decoded := []gopacket.LayerType{}
	if err := parser.DecodeLayers(sndpkt.packet.Data(), &decoded); err != nil {
		log.Warnf("Unable to decode packet from %s: %s", sndpkt.srcif, err)
		return
	}

	// packet was decoded
	found_udp := false
	found_ipv4 := false
	for _, layerType := range decoded {
		switch layerType {
		case layers.LayerTypeUDP:
			found_udp = true
		case layers.LayerTypeIPv4:
			found_ipv4 = true
		}
	}
	if !found_udp || !found_ipv4 {
		log.Warnf("Packet from %s did not contain a IPv4/UDP packet", sndpkt.srcif)
		return
	}

	// Build our packet to send
	buffer := gopacket.NewSerializeBuffer()
	csum_opts := gopacket.SerializeOptions{
		FixLengths:       false,
		ComputeChecksums: true, // only works for IPv4
	}
	opts := gopacket.SerializeOptions{
		FixLengths:       false,
		ComputeChecksums: false,
	}

	// UDP payload
	if err := payload.SerializeTo(buffer, opts); err != nil {
		log.Fatalf("can't serialize payload: %v", payload)
	}

	// UDP checksums can't be calculated via SerializeOptions
	// because it requires the IP pseudo-header:
	// https://en.wikipedia.org/wiki/User_Datagram_Protocol#IPv4_pseudo_header
	new_udp := layers.UDP{
		SrcPort:  udp.SrcPort,
		DstPort:  udp.DstPort,
		Checksum: 0, // but 0 is always valid for UDP
		Length:   uint16(8 + len(payload)),
	}

	if err := new_udp.SerializeTo(buffer, opts); err != nil {
		log.Fatalf("can't serialize UDP header: %v", udp)
	}

	// IPv4 header
	new_ip4 := layers.IPv4{
		Version:    ip4.Version,
		IHL:        ip4.IHL,
		TOS:        ip4.TOS,
		Length:     ip4.Length,
		Id:         ip4.Id,
		Flags:      ip4.Flags,
		FragOffset: ip4.FragOffset,
		TTL:        ip4.TTL,
		Protocol:   ip4.Protocol,
		Checksum:   0, // reset to calc checksums
		SrcIP:      ip4.SrcIP,
		DstIP:      net.ParseIP(l.ipaddr).To4(),
		Options:    ip4.Options,
	}
	if err := new_ip4.SerializeTo(buffer, csum_opts); err != nil {
		log.Fatalf("can't serialize IP header: %v", new_ip4)
	}

	// Loopback or Ethernet
	if (l.iface.Flags & net.FlagLoopback) > 0 {
		loop := layers.Loopback{
			Family: layers.ProtocolFamilyIPv4,
		}
		if err := loop.SerializeTo(buffer, opts); err != nil {
			log.Fatalf("can't serialize Loop header: %v", loop)
		}
	} else {
		// build a new ethernet header
		new_eth := layers.Ethernet{
			BaseLayer:    layers.BaseLayer{},
			DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			SrcMAC:       l.iface.HardwareAddr,
			EthernetType: eth.EthernetType,
		}
		if err := new_eth.SerializeTo(buffer, opts); err != nil {
			log.Fatalf("can't serialize Eth header: %v", new_eth)
		}
	}

	outgoingPacket := buffer.Bytes()
	log.Debugf("%s: packet len: %d: %v", l.iname, len(outgoingPacket), outgoingPacket)
	err := l.handle.WritePacketData(outgoingPacket)
	if err != nil {
		log.Warnf("Unable to send %d bytes from %s out %s: %s",
			len(outgoingPacket), sndpkt.srcif, l.iname, err)
	}
}

// Returns if the provided layertype is valid
func isValidLayerType(layertype layers.LinkType) bool {
	for _, b := range validLinkTypes {
		if b == layertype {
			return true
		}
	}
	return false
}
