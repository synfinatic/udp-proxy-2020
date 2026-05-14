package proxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
)

// DeviceManager handles discovery and initialization of network interfaces.
type DeviceManager struct {
	mu         sync.RWMutex
	interfaces map[string]pcap.Interface
}

// NewDeviceManager creates and initializes a new DeviceManager.
func NewDeviceManager() (*DeviceManager, error) {
	dm := &DeviceManager{
		interfaces: make(map[string]pcap.Interface),
	}
	if err := dm.Refresh(); err != nil {
		return nil, err
	}
	return dm, nil
}

// Refresh updates the list of available devices.
func (dm *DeviceManager) Refresh() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	ifs, err := pcap.FindAllDevs()
	if err != nil {
		return err
	}

	// Reset map
	dm.interfaces = make(map[string]pcap.Interface)
	for _, i := range ifs {
		if len(i.Addresses) == 0 {
			continue
		}
		dm.interfaces[i.Name] = i
	}
	return nil
}

// GetAddresses returns the addresses for a specific interface.
func (dm *DeviceManager) GetAddresses(iname string) ([]pcap.InterfaceAddress, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if dev, ok := dm.interfaces[iname]; ok {
		return dev.Addresses, nil
	}
	return nil, fmt.Errorf("interface %s not found or has no addresses", iname)
}

// CreateHandle initializes a libpcap handle for the given interface.
func (dm *DeviceManager) CreateHandle(iname string, promisc bool, timeout time.Duration) (*pcap.Handle, error) {
	inactive, err := pcap.NewInactiveHandle(iname)
	if err != nil {
		return nil, err
	}
	defer inactive.CleanUp()

	if err = inactive.SetTimeout(timeout); err != nil {
		return nil, err
	}
	if err = inactive.SetPromisc(promisc); err != nil {
		return nil, err
	}
	if err = inactive.SetSnapLen(9000); err != nil {
		return nil, err
	}

	handle, err := inactive.Activate()
	if err != nil {
		return nil, err
	}

	if !dm.isValidLinkType(handle.LinkType()) {
		handle.Close()
		return nil, fmt.Errorf("interface %s has an unsupported link type: %s", iname, handle.LinkType())
	}

	// Default to inbound only
	if err := handle.SetDirection(pcap.DirectionIn); err != nil {
		handle.Close()
		return nil, fmt.Errorf("failed to set direction on interface %s: %w", iname, err)
	}

	return handle, nil
}

func (dm *DeviceManager) isValidLinkType(lt layers.LinkType) bool {
	switch lt {
	case layers.LinkTypeLoop, layers.LinkTypeEthernet, layers.LinkTypeNull, layers.LinkTypeRaw:
		return true
	}
	return false
}

// ListInterfaces prints available network interfaces.
func (dm *DeviceManager) ListInterfaces() {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	for k, v := range dm.interfaces {
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

// GetLoopback returns the name of the loopback interface.
func (dm *DeviceManager) GetLoopback() string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	for k, v := range dm.interfaces {
		for _, a := range v.Addresses {
			if a.IP.IsLoopback() {
				return k
			}
		}
	}
	return ""
}
