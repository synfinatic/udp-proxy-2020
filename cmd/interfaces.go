package main

import (
	"fmt"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
	"net"
)

// var Timeout time.Duration
var Interfaces = map[string]pcap.Interface{}

func initalizeInterface(l *Listen) {
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

	if !isValidLayerType(l.handle.LinkType()) {
		log.Fatalf("%s: has an invalid layer type: 0x%02x", l.iface, l.handle.LinkType())
	}

	// set our BPF filter
	log.Debugf("%s: applying BPF Filter: %s", l.iface, l.filter)
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

	// create the raw socket to send UDP messages
	for i, ip := range Interfaces[l.iface].Addresses {
		// first, figure out out IPv4 address
		if net.IP.To4(ip.IP) == nil {
			log.Debugf("\tskipping %d: %s", i, ip.IP.String())
			continue
		}
		log.Debugf("%s: %s", l.iface, ip.IP.String())

		// create our ip:udp socket
		listen := fmt.Sprintf("%s", ip.IP.String())
		u, err = net.ListenPacket("ip:udp", listen) // don't close this
		if err != nil {
			log.Fatalf("%s: %s", l.iface, err)
		}
		log.Debugf("%s: listening on %s", l.iface, listen)
		break
	}

	// make sure we create our ip:udp socket
	if u == nil {
		log.Fatalf("%s: No IPv4 address configured. Unable to listen for UDP.", l.iface)
	}

	// use that ip:udp socket to create a new raw socket
	p := ipv4.NewPacketConn(u) // don't close this

	if l.raw, err = ipv4.NewRawConn(u); err != nil {
		log.Fatalf("%s: %s", l.iface, err)
	}
	log.Debugf("Opened raw socket on %s: %s", l.iface, p.LocalAddr().String())
}

// Uses libpcap to get a list of configured interfaces
// and populate the Interfaces.
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

// Print out a list of all the interfaces that libpcap sees
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
