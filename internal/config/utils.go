package config

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gopacket/gopacket/pcap"
)

// BuildBPFFilter takes a list of ports and builds a BPF filter string.
func BuildBPFFilter(ports []int32, addresses []pcap.InterfaceAddress) string {
	if len(ports) < 1 {
		return ""
	}
	var bpfFilters = []string{}
	for _, p := range ports {
		bpfFilters = append(bpfFilters, fmt.Sprintf("udp port %d", p))
	}
	var bpfFilter string
	if len(ports) > 1 {
		bpfFilter = strings.Join(bpfFilters, " or ")
	} else {
		bpfFilter = bpfFilters[0]
	}

	networks := []string{}
	for _, addr := range addresses {
		if netStr, err := GetNetwork(addr); err == nil {
			if maskLen, _ := addr.Netmask.Size(); maskLen > 0 {
				networks = append(networks, fmt.Sprintf("src net %s", netStr))
			}
		}
	}
	if len(networks) >= 1 {
		networkFilter := strings.Join(networks, " or ")
		bpfFilter = fmt.Sprintf("(%s) and (%s)", bpfFilter, networkFilter)
	}

	return bpfFilter
}

// ParseTimeout converts a millisecond timeout into a time.Duration.
func ParseTimeout(timeout int64) time.Duration {
	return time.Duration(timeout) * time.Millisecond
}

// GetNetwork takes a pcap.InterfaceAddress and returns a CIDR x.x.x.x/len format.
func GetNetwork(addr pcap.InterfaceAddress) (string, error) {
	var ip4 net.IP
	if ip4 = addr.IP.To4(); ip4 == nil {
		return "", fmt.Errorf("unable to getNetwork for IPv6 address: %s", addr.IP.String())
	}

	size, _ := addr.Netmask.Size()
	mask := net.CIDRMask(size, 32)
	return fmt.Sprintf("%s/%d", ip4.Mask(mask), size), nil
}
