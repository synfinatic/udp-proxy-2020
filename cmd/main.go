package main

import (
	"fmt"
	//	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"golang.org/x/net/ipv4"
	"net"
	"os"
	"os/user"
	"sync"
	"time"
)

type Listen struct {
	iface   string
	filter  string
	ports   []int32
	promisc bool
	handle  *pcap.Handle
	raw     *ipv4.RawConn
	timeout time.Duration
	pkts    chan []byte
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
	if err = inactive.SetTimeout(l.timeout); err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	} else if err = inactive.SetPromisc(l.promisc); err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	} else if err = inactive.SetSnapLen(9000); err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}

	// activate libpcap handle
	if l.handle, err = inactive.Activate(); err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}

	// set our BPF filter
	if err = l.handle.SetBPFFilter(l.filter); err != nil {
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

// Goroutine to watch interfaces
func watch_interface(listen Listen, listeners []Listen, wg *sync.WaitGroup) {
	// listen on our pcap handler ...

	// now send UDP packet out the other UDP sockets
	for _, other := range listeners {
		log.Debug(other)
	}
}

func main() {
	var listen = []string{}
	var promisc = []string{}
	var ports = []int32{}
	var timeout int64
	var debug bool
	var ilist bool

	// option parsing
	flag.StringArrayVar(&listen, "listen", []string{}, "zero or more non-promisc interfaces")
	flag.StringArrayVar(&promisc, "promisc", []string{}, "zero or more promiscuous interfaces")
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
	for _, iface := range listen {
		if stringInSlice(iface, interfaces) {
			log.Fatalf("Can't specify the same interface (%s) multiple times", iface)
		}
		interfaces = append(interfaces, iface)
		new := Listen{
			iface:   iface,
			filter:  bpf_filter,
			ports:   ports,
			timeout: to,
			promisc: false,
			handle:  nil,
			raw:     nil,
		}
		Listeners = append(Listeners, new)
	}
	for _, iface := range promisc {
		if stringInSlice(iface, interfaces) {
			log.Fatalf("Can't specify the same interface (%s) multiple times", iface)
		}
		interfaces = append(interfaces, iface)
		new := Listen{
			iface:   iface,
			filter:  bpf_filter,
			ports:   ports,
			timeout: to,
			promisc: false,
			handle:  nil,
			raw:     nil,
		}
		Listeners = append(Listeners, new)
	}

	// init each listener
	for _, l := range Listeners {
		initalizeInterface(l)
		defer l.handle.Close()
	}

	// start handling packets
	var wg sync.WaitGroup
	log.Debug("Initialization complete!")
	for _, l := range Listeners {
		wg.Add(1)
		go watch_interface(l, Listeners, &wg)
	}
	wg.Wait()
}
