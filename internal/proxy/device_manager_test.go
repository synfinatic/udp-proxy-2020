package proxy

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket/pcap"
)

func TestDeviceManager_GetLoopback(t *testing.T) {
	dm := &DeviceManager{
		interfaces: make(map[string]pcap.Interface),
	}

	// Mock loopback
	dm.interfaces["lo0"] = pcap.Interface{
		Name: "lo0",
		Addresses: []pcap.InterfaceAddress{
			{IP: net.ParseIP("127.0.0.1")},
		},
	}
	// Mock ethernet
	dm.interfaces["eth0"] = pcap.Interface{
		Name: "eth0",
		Addresses: []pcap.InterfaceAddress{
			{IP: net.ParseIP("192.168.1.1")},
		},
	}

	lb := dm.GetLoopback()
	if lb != "lo0" {
		t.Errorf("Expected loopback lo0, got %s", lb)
	}
}

func TestDeviceManager_GetAddresses(t *testing.T) {
	dm := &DeviceManager{
		interfaces: make(map[string]pcap.Interface),
	}

	addr := pcap.InterfaceAddress{IP: net.ParseIP("192.168.1.1")}
	dm.interfaces["eth0"] = pcap.Interface{
		Name:      "eth0",
		Addresses: []pcap.InterfaceAddress{addr},
	}

	addrs, err := dm.GetAddresses("eth0")
	if err != nil {
		t.Fatalf("GetAddresses failed: %v", err)
	}
	if len(addrs) != 1 || addrs[0].IP.String() != "192.168.1.1" {
		t.Errorf("Unexpected addresses: %v", addrs)
	}

	_, err = dm.GetAddresses("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent interface")
	}
}
