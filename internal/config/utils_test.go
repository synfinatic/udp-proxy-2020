package config

import (
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket/pcap"
)

func TestParseTimeout(t *testing.T) {
	got := ParseTimeout(250)
	want := 250 * time.Millisecond
	if got != want {
		t.Errorf("ParseTimeout(250) = %v; want %v", got, want)
	}
}

func TestGetNetwork(t *testing.T) {
	tests := []struct {
		name    string
		addr    pcap.InterfaceAddress
		want    string
		wantErr bool
	}{
		{
			name: "standard ipv4",
			addr: pcap.InterfaceAddress{
				IP:      net.IP{192, 168, 1, 10},
				Netmask: net.IPMask{255, 255, 255, 0},
			},
			want: "192.168.1.0/24",
		},
		{
			name: "ipv6 failure",
			addr: pcap.InterfaceAddress{
				IP: net.ParseIP("2001:db8::1"),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetNetwork(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNetwork() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetNetwork() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildBPFFilter(t *testing.T) {
	addr := pcap.InterfaceAddress{
		IP:      net.IP{192, 168, 1, 10},
		Netmask: net.IPMask{255, 255, 255, 0},
	}
	
	got := BuildBPFFilter([]int32{53, 67}, []pcap.InterfaceAddress{addr})
	want := "(udp port 53 or udp port 67) and (src net 192.168.1.0/24)"
	if got != want {
		t.Errorf("BuildBPFFilter() = %q, want %q", got, want)
	}
}
