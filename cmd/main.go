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
	"strings"
	"time"
)

type Listen struct {
	iface   string
	filter  string
	ports   []int32
	promisc bool
	handle  *pcap.Handle
	raw     *ipv4.RawConn
}

var Timeout time.Duration
var Listeners []Listen
var Interfaces = map[string]pcap.Interface{}

func getConfiguredInterfaces() {
	if len(Interfaces) > 0 {
		return
	}
	ifs, err := pcap.FindAllDevs()
	if err != nil {
		log.Fatal(err)
	}
	for _, i := range ifs {
		if len(i.Addresses) == 0 {
			continue
		}
		Interfaces[i.Name] = i
	}
}

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
	if err = inactive.SetTimeout(Timeout); err != nil {
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

func listInterfaces() {
	getConfiguredInterfaces()
	for k, v := range Interfaces {
		fmt.Printf("Interface: %s\n", k)
		for _, a := range v.Addresses {
			ones, _ := a.Netmask.Size()
			if a.Broadaddr != nil {
				fmt.Printf("\t- IP: %s/%d  Broadaddr: %s\n",
					a.IP.String(), ones, a.Broadaddr.String())
			} else if a.P2P != nil {
				fmt.Printf("\t- IP: %s/%d  PointToPoint: %s\n",
					a.IP.String(), ones, a.P2P.String())
			} else {
				fmt.Printf("\t- IP: %s/%d\n", a.IP.String(), ones)
			}
		}
		fmt.Printf("\n")
	}
}

func main() {
	var listen = []string{}
	var promisc = []string{}
	var ports = []int32{}
	var timeout int64
	var debug bool
	var ilist bool

	flag.StringArrayVar(&listen, "listen", []string{}, "zero or more non-promisc interfaces")
	flag.StringArrayVar(&promisc, "promisc", []string{}, "zero or more promiscuous interfaces")
	flag.Int32SliceVar(&ports, "port", []int32{}, "one or UDP ports to process")
	flag.Int64Var(&timeout, "timeout", 250, "timeout in ms")
	flag.BoolVar(&debug, "debug", false, "Enable debugging")
	flag.BoolVar(&ilist, "list-interfaces", false, "List interfaces and exit")

	flag.Parse()

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
	u, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	if u.Uid != "0" {
		log.Fatal("need to run as root in order for raw sockets to work")
	}

	d := fmt.Sprintf("%dms", timeout)
	Timeout, err := time.ParseDuration(d)
	if err != nil {
		log.Fatal(err)
	}

	total := len(promisc) + len(listen)
	if total < 2 {
		log.Fatal("Please specify --promisc and --listen at least twice in total")
	}

	if len(ports) < 1 {
		log.Fatal("--port must be specified one or more times")
	}

	var bpf_filters = []string{}
	for _, p := range ports {
		bpf_filters = append(bpf_filters, fmt.Sprintf("udp port %d", p))
	}
	var bpf_filter string
	if len(ports) > 1 {
		bpf_filter = strings.Join(bpf_filters, " or ")
	} else {
		bpf_filter = bpf_filters[0]
	}

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
			promisc: false,
			handle:  nil,
			raw:     nil,
		}
		Listeners = append(Listeners, new)
	}

	for _, l := range Listeners {
		initalizeInterface(l)
		defer l.handle.Close()
	}
	fmt.Printf("%v\n", Timeout)
}
