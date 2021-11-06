package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
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

// takes a list of ports and builds our BPF filter
func buildBPFFilter(ports []int32, addresses []pcap.InterfaceAddress, promisc bool) string {
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

	// add filter to accept only traffic with a src IP matching the interface
	// This should avoid network loops with NIC/drivers which do not honor the
	// pcap.SetDirection() call.
	networks := []string{}
	for _, addr := range addresses {
		if net, err := getNetwork(addr); err == nil {
			if maskLen, _ := addr.Netmask.Size(); maskLen > 0 {
				networks = append(networks, fmt.Sprintf("src net %s", net))
			}
		}
	}
	var networkFilter string
	if len(networks) >= 1 {
		networkFilter = strings.Join(networks, " or ")
		bpf_filter = fmt.Sprintf("(%s) and (%s)", bpf_filter, networkFilter)
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

// takes a net.IP and returns x.x.x.x/len format
func getNetwork(addr pcap.InterfaceAddress) (string, error) {
	var ip4 net.IP
	if ip4 = addr.IP.To4(); ip4 == nil {
		return "", fmt.Errorf("Unable to getNetwork for IPv6 address: %s", addr.IP.String())
	}

	len, _ := addr.Netmask.Size()
	mask := net.CIDRMask(len, 32)
	return fmt.Sprintf("%s/%d", ip4.Mask(mask), len), nil
}
