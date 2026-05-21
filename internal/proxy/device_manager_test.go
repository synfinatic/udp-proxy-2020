package proxy

import (
	"net"
	"runtime"
	"testing"

	"github.com/gopacket/gopacket/layers"
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

func TestDeviceManager_isValidLinkType(t *testing.T) {
	dm := &DeviceManager{}

	alwaysValid := []layers.LinkType{
		layers.LinkTypeLoop,
		layers.LinkTypeEthernet,
		layers.LinkTypeNull,
		layers.LinkTypeRaw,
	}
	for _, lt := range alwaysValid {
		if !dm.isValidLinkType(lt) {
			t.Errorf("expected link type %v to be valid on all platforms", lt)
		}
	}

	if dm.isValidLinkType(layers.LinkType(255)) {
		t.Error("expected unknown link type 255 to be invalid")
	}

	// LinkTypeRawOpenBSD is only valid on OpenBSD.
	gotOpenBSD := dm.isValidLinkType(LinkTypeRawOpenBSD)
	wantOpenBSD := runtime.GOOS == "openbsd"
	if gotOpenBSD != wantOpenBSD {
		t.Errorf("isValidLinkType(LinkTypeRawOpenBSD) = %v, want %v (GOOS=%s)", gotOpenBSD, wantOpenBSD, runtime.GOOS)
	}

	// LinkTypeRawOthers is valid on every OS except OpenBSD.
	gotOthers := dm.isValidLinkType(LinkTypeRawOthers)
	wantOthers := runtime.GOOS != "openbsd"
	if gotOthers != wantOthers {
		t.Errorf("isValidLinkType(LinkTypeRawOthers) = %v, want %v (GOOS=%s)", gotOthers, wantOthers, runtime.GOOS)
	}
}
