package main

import (
	"encoding/binary"
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
)

const SendBufferSize = 100

// Struct containing everything for an interface
type Listen struct {
	iname     string               // interface to use
	netif     *net.Interface       // interface descriptor
	ports     []int32              // port(s) we listen for packets
	ipaddr    string               // dstip we send packets to
	promisc   bool                 // do we enable promisc on this interface?
	handle    *pcap.Handle         // gopacket.pcap handle
	timeout   time.Duration        // timeout for loop
	clientTTL time.Duration        // ttl for client cache
	sendpkt   chan Send            // channel used to recieve packets we need to send
	clients   map[string]time.Time // keep track of clients for non-promisc interfaces
}

// List of LayerTypes we support in sendPacket()
var validLinkTypes = []layers.LinkType{
	layers.LinkTypeLoop,
	layers.LinkTypeEthernet,
	layers.LinkTypeNull,
	layers.LinkTypeRaw,
}

// Creates a Listen struct for the given interface, promisc mode, udp sniff ports and timeout
func newListener(netif *net.Interface, promisc bool, ports []int32, to time.Duration, fixed_ip []string) Listen {
	log.Debugf("%s: ifIndex: %d", netif.Name, netif.Index)
	addrs, err := netif.Addrs()
	if err != nil {
		log.Fatalf("Unable to obtain addresses for %s", netif.Name)
	}
	var bcastaddr string = ""
	// only calc the broadcast address on promiscuous interfaces
	// for non-promisc, we use our clients
	if !promisc {
		for _, addr := range addrs {
			log.Debugf("%s network: %s\t\tstring: %s", netif.Name, addr.Network(), addr.String())

			_, ipNet, err := net.ParseCIDR(addr.String())
			if err != nil {
				log.Debugf("%s: Unable to parse CIDR: %s (%s)", netif.Name, addr.String(), addr.Network())
				continue
			}
			if ipNet.IP.To4() == nil {
				continue // Skip non-IPv4 addresses
			}
			// calc broadcast
			ip := make(net.IP, len(ipNet.IP.To4()))
			bcastbin := binary.BigEndian.Uint32(ipNet.IP.To4()) | ^binary.BigEndian.Uint32(net.IP(ipNet.Mask).To4())
			binary.BigEndian.PutUint32(ip, bcastbin)
			bcastaddr = ip.String()
		}
		// promisc interfaces should have a bcast/ipv4 config
		if len(bcastaddr) == 0 && promisc {
			log.Fatalf("%s does not have a valid IPv4 configuration", netif.Name)
		}
	}

	// fixed ip clients
	clients := make(map[string]time.Time)
	for _, ip := range fixed_ip {
		clients[ip] = time.Time{} // zero value
	}

	new := Listen{
		iname:   netif.Name,
		netif:   netif,
		ports:   ports,
		ipaddr:  bcastaddr,
		timeout: to,
		promisc: promisc,
		handle:  nil,
		sendpkt: make(chan Send, SendBufferSize),
		clients: clients,
	}
	log.Debugf("Listen: %v", new)
	return new
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
			l.sendPackets(s)
		case packet := <-packets: // packet arrived on this interfaces
			// is it legit?
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil || packet.TransportLayer().LayerType() != layers.LayerTypeUDP {
				log.Warnf("%s: Invalid packet", l.iname)
				continue
			} else if errx := packet.ErrorLayer(); errx != nil {
				log.Errorf("%s: Unable to decode: %s", l.iname, errx.Error())
			}

			// if our interface is non-promisc, learn the client IP
			if l.promisc {
				l.learnClientIP(packet)
			}

			log.Debugf("%s: received packet and fowarding onto other interfaces", l.iname)
			s.Send(packet, l.iname, l.handle.LinkType())
		case <-ticker: // our timer
			log.Debugf("handlePackets(%s) ticker", l.iname)
			// clean client cache
			for k, v := range l.clients {
				// zero is hard code values
				if !v.IsZero() && v.Before(time.Now()) {
					log.Debugf("%s removing %s after %dsec", l.iname, k, l.clientTTL)
					delete(l.clients, k)
				}
			}
		}
	}
}

// Does the heavy lifting of editing & sending the packet onwards
func (l *Listen) sendPackets(sndpkt Send) {
	var eth layers.Ethernet
	var loop layers.Loopback // BSD NULL/Loopback used for OpenVPN tunnels/etc
	var ip4 layers.IPv4      // we only support v4
	var udp layers.UDP
	var payload gopacket.Payload
	var parser *gopacket.DecodingLayerParser

	log.Debugf("processing packet from %s on %s", sndpkt.srcif, l.iname)

	switch sndpkt.linkType.String() {
	case layers.LinkTypeNull.String(), layers.LinkTypeLoop.String():
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeLoopback, &loop, &ip4, &udp, &payload)
	case layers.LinkTypeEthernet.String():
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &udp, &payload)
	case layers.LinkTypeRaw.String():
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &ip4, &udp, &payload)
	default:
		log.Fatalf("Unsupported source linktype: %s", sndpkt.linkType.String())
	}

	// try decoding our packet
	decoded := []gopacket.LayerType{}
	if err := parser.DecodeLayers(sndpkt.packet.Data(), &decoded); err != nil {
		log.Warnf("Unable to decode packet from %s: %s", sndpkt.srcif, err)
		return
	}

	// was packet decoded?  In theory, this should never happen because our BPF filter...
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

	if !l.promisc {
		// send one packet to broadcast IP
		dstip := net.ParseIP(l.ipaddr).To4()
		if err, bytes := l.sendPacket(dstip, eth, loop, ip4, udp, payload); err != nil {
			log.Warnf("Unable to send %d bytes from %s out %s: %s",
				bytes, sndpkt.srcif, l.iname, err)
		}
	} else {
		// sent packet to every client
		if len(l.clients) == 0 {
			log.Debugf("%s: Unable to send packet; no discovered clients", l.iname)
		}
		for ip, _ := range l.clients {
			dstip := net.ParseIP(ip).To4()
			if err, bytes := l.sendPacket(dstip, eth, loop, ip4, udp, payload); err != nil {
				log.Warnf("Unable to send %d bytes from %s out %s: %s",
					bytes, sndpkt.srcif, l.iname, err)
			}
		}
	}
}

func (l *Listen) sendPacket(dstip net.IP, eth layers.Ethernet, loop layers.Loopback,
	ip4 layers.IPv4, udp layers.UDP, payload gopacket.Payload) (error, int) {
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
		DstIP:      dstip,
		Options:    ip4.Options,
	}
	if err := new_ip4.SerializeTo(buffer, csum_opts); err != nil {
		log.Fatalf("can't serialize IP header: %v", new_ip4)
	}

	// Add our L2 header to the buffer
	switch l.handle.LinkType().String() {
	case layers.LinkTypeNull.String(), layers.LinkTypeLoop.String():
		loop := layers.Loopback{
			Family: layers.ProtocolFamilyIPv4,
		}
		if err := loop.SerializeTo(buffer, opts); err != nil {
			log.Fatalf("can't serialize Loop header: %v", loop)
		}
	case layers.LinkTypeEthernet.String():
		// build a new ethernet header
		new_eth := layers.Ethernet{
			BaseLayer:    layers.BaseLayer{},
			DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			SrcMAC:       l.netif.HardwareAddr,
			EthernetType: layers.EthernetTypeIPv4,
		}
		if err := new_eth.SerializeTo(buffer, opts); err != nil {
			log.Fatalf("can't serialize Eth header: %v", new_eth)
		}
	case layers.LinkTypeRaw.String():
		// no L2 header
	default:
		log.Warnf("Unsupported linktype: %s", l.handle.LinkType().String())
	}

	outgoingPacket := buffer.Bytes()
	log.Debugf("%s => %s: packet len: %d: %s",
		l.iname, dstip.String(), len(outgoingPacket), hex.EncodeToString(outgoingPacket))
	return l.handle.WritePacketData(outgoingPacket), len(outgoingPacket)
}

func (l *Listen) learnClientIP(packet gopacket.Packet) {
	var eth layers.Ethernet
	var loop layers.Loopback
	var ip4 layers.IPv4
	var udp layers.UDP
	var payload gopacket.Payload
	var parser *gopacket.DecodingLayerParser

	switch l.handle.LinkType().String() {
	case layers.LinkTypeNull.String(), layers.LinkTypeLoop.String():
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeLoopback, &loop, &ip4, &udp, &payload)
	case layers.LinkTypeEthernet.String():
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &udp, &payload)
	case layers.LinkTypeRaw.String():
		parser = gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &ip4, &udp, &payload)
	default:
		log.Fatalf("Unsupported source linktype: %s", l.handle.LinkType().String())
	}

	decoded := []gopacket.LayerType{}
	if err := parser.DecodeLayers(packet.Data(), &decoded); err != nil {
		log.Debugf("Unable to decoded client IP on %s: %s", l.iname, err)
	}

	found_ipv4 := false
	for _, layerType := range decoded {
		switch layerType {
		case layers.LayerTypeIPv4:
			// found our v4 header
			found_ipv4 = true
		}
	}

	if found_ipv4 {
		val, exists := l.clients[ip4.SrcIP.String()]
		if !exists || !val.IsZero() {
			l.clients[ip4.SrcIP.String()] = time.Now().Add(l.clientTTL)
			log.Debugf("%s: Learned client IP: %s", l.iname, ip4.SrcIP.String())
		}
	}
}

// Returns if the provided layertype is valid
func isValidLayerType(layertype layers.LinkType) bool {
	for _, b := range validLinkTypes {
		if strings.Compare(b.String(), layertype.String()) == 0 {
			return true
		}
	}
	return false
}
