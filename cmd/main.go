package main

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"golang.org/x/net/ipv4"
	"net"
	"os"
	"os/user"
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

type Send struct {
	packet gopacket.Packet // packet data
	srcif  string          // interface it came in on
}

type SendPktFeed struct {
	lock    sync.Mutex  // lock
	senders []chan Send // list of channels to send packets on
}

// Function to send a packet out all the other interfaces other than srcif
func (s *SendPktFeed) Send(p gopacket.Packet, srcif string) {
	s.lock.Lock()
	for _, send := range s.senders {
		send <- Send{packet: p, srcif: srcif}
	}
	s.lock.Unlock()
}

// Register a channel to recieve packet data we want to send
func (s *SendPktFeed) RegisterSender(send chan Send) {
	s.lock.Lock()
	s.senders = append(s.senders, send)
	s.lock.Unlock()
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

var Timeout time.Duration
var Listeners []Listen
var Interfaces = map[string]pcap.Interface{}

func initalizeInterface(l Listen) {
	// find our interface via libpcap
	getConfiguredInterfaces()
	if len(Interfaces[l.iface].Addresses) == 0 {
		log.Fatalf("%s is not configured")
	}

	// configure libpcap listener
	inactive, err := pcap.NewInactiveHandle(l.iface)
	if err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}
	defer inactive.CleanUp()

	// set our timeout
	err = inactive.SetTimeout(l.timeout)
	if err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}
	// Promiscuous mode on/off
	err = inactive.SetPromisc(l.promisc)
	if err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}
	// Get the entire packet
	err = inactive.SetSnapLen(9000)
	if err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}

	// activate libpcap handle
	if l.handle, err = inactive.Activate(); err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}

	// set our BPF filter
	err = l.handle.SetBPFFilter(l.filter)
	if err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}

	// just inbound packets
	err = l.handle.SetDirection(pcap.DirectionIn)
	if err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}

	log.Debugf("Opened pcap handle on %s", l.iface)
	var u net.PacketConn = nil
	var listen string

	// create the raw socket to send UDP messages
	for _, ip := range Interfaces[l.iface].Addresses {
		// first, figure out out IPv4 address
		if net.IP.To4(ip.IP) == nil {
			continue
		}
		log.Debugf("%s: %s", l.iface, ip.IP.String())

		// create our ip:udp socket
		listen = fmt.Sprintf("%s", ip.IP.String())
		u, err = net.ListenPacket("ip:udp", listen)
		if err != nil {
			log.Fatalf("%s: %s", l.iface, err)
		}
		log.Debugf("%s: listening on %s", l.iface, listen)
		defer u.Close()
		break
	}

	// make sure we create our ip:udp socket
	if u == nil {
		log.Fatalf("%s: Unable to figure out where to listen for UDP", l.iface)
	}

	// use that ip:udp socket to create a new raw socket
	p := ipv4.NewPacketConn(u)
	defer p.Close()

	if l.raw, err = ipv4.NewRawConn(u); err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}
	log.Debugf("Opened raw socket on %s: %s", l.iface, p.LocalAddr().String())
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

func main() {
	var listen = []string{}
	var promisc = []string{}
	var ports = []int32{}
	var timeout int64
	var debug bool
	var ilist bool

	// option parsing
	flag.StringArrayVar(&listen, "listen", []string{}, "zero or more non-promisc interface@dstip")
	flag.StringArrayVar(&promisc, "promisc", []string{}, "zero or more promiscuous interface@dstip")
	flag.Int32SliceVar(&ports, "port", []int32{}, "one or more UDP ports to process")
	flag.Int64Var(&timeout, "timeout", 250, "timeout in ms")
	flag.BoolVar(&debug, "debug", false, "Enable debugging")
	flag.BoolVar(&ilist, "list-interfaces", false, "List available interfaces and exit")

	flag.Parse()

	// turn on debugging?
	if debug == true {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	if ilist == true {
		listInterfaces()
		os.Exit(0)
	}

	// make sure we're root
	if u, err := user.Current(); err != nil {
		log.Fatal(err)
	} else if u.Uid != "0" {
		log.Fatal("need to run as root in order for raw sockets to work")
	}

	// Neeed at least two interfaces
	total := len(promisc) + len(listen)
	if total < 2 {
		log.Fatal("Please specify --promisc and --listen at least twice in total")
	}

	// handle our timeout & bpf filter for ports
	to := parseTimeout(timeout)
	bpf_filter := buildBPFFilter(ports)

	// process our promisc and listen interfaces
	var interfaces = []string{}
	a := processListener(&interfaces, listen, false, bpf_filter, ports, to)
	for _, x := range a {
		Listeners = append(Listeners, x)
	}
	a = processListener(&interfaces, promisc, true, bpf_filter, ports, to)
	for _, x := range a {
		Listeners = append(Listeners, x)
	}

	// init each listener
	for _, l := range Listeners {
		initalizeInterface(l)
		defer l.handle.Close()
	}

	// start handling packets
	var wg sync.WaitGroup
	spf := SendPktFeed{}
	log.Debug("Initialization complete!")
	for _, l := range Listeners {
		wg.Add(1)
		go l.handlePackets(&spf, &wg)
	}
	wg.Wait()
}
