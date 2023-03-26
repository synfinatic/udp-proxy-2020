package main

import (
	"fmt"

	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
)

// Interfaces is a map between interface name and pcap data structure
var Interfaces = map[string]pcap.Interface{}

func initializeInterface(l *Listen) {
	// find our interface via libpcap
	getConfiguredInterfaces()
	if len(Interfaces[l.iname].Addresses) == 0 {
		log.Fatalf("%s is not configured", l.iname)
	}

	// configure libpcap listener
	inactive, err := pcap.NewInactiveHandle(l.iname)
	if err != nil {
		log.Fatalf("%s: %s", l.iname, err)
	}
	defer inactive.CleanUp()

	// set our timeout
	if err = inactive.SetTimeout(l.timeout); err != nil {
		log.Fatalf("%s: %s", l.iname, err)
	}

	// Promiscuous mode on/off
	if err = inactive.SetPromisc(l.promisc); err != nil {
		log.Fatalf("%s: %s", l.iname, err)
	}
	// Get the entire packet
	if err = inactive.SetSnapLen(9000); err != nil {
		log.Fatalf("%s: %s", l.iname, err)
	}

	// activate libpcap handle
	if l.handle, err = inactive.Activate(); err != nil {
		log.Fatalf("%s: %s", l.iname, err)
	}

	if !isValidLayerType(l.handle.LinkType()) {
		log.Fatalf("%s: has an invalid layer type: %s", l.iname, l.handle.LinkType().String())
	}

	// set our BPF filter
	bpf_filter := buildBPFFilter(l.ports, Interfaces[l.iname].Addresses, l.promisc)
	log.Debugf("%s: applying BPF Filter: %s", l.iname, bpf_filter)
	if err = l.handle.SetBPFFilter(bpf_filter); err != nil {
		log.Fatalf("%s: %s", l.iname, err)
	}

	// just inbound packets
	if err = l.handle.SetDirection(pcap.DirectionIn); err != nil {
		log.Fatalf("%s: %s", l.iname, err)
	}

	log.Debugf("Opened pcap handle on %s", l.iname)
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

// getLoopback returns the name of the loopback interface
func getLoopback() string {
	getConfiguredInterfaces()
	for k, v := range Interfaces {
		for _, a := range v.Addresses {
			if a.IP.String() == "127.0.0.1" {
				return k
			}
		}
	}
	return "No Loopback Interface"
}
