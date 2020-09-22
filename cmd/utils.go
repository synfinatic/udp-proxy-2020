package main

import (
	"fmt"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

// Check to see if the string is in the slice
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// Check to see if the string prefix is in the slice
func stringPrefixInSlice(a string, list []string) bool {
	for _, b := range list {
		if strings.HasPrefix(b, a) {
			return true
		}
	}
	return false
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

// takes a list of ports and builds our BPF filter
func buildBPFFilter(ports []int32) string {
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
	return bpf_filter
}

func parseTimeout(timeout int64) time.Duration {
	d := fmt.Sprintf("%dms", timeout)
	to, err := time.ParseDuration(d)
	if err != nil {
		log.Fatal(err)
	}
	return to
}
