package main

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"
)

// startUDPListeners starts a UDP listener on each interface and port, reading and discarding packets.
func startUDPListeners(ctx context.Context, wg *sync.WaitGroup, cli CLI) {
	for _, iname := range cli.Interface {
		addrs, err := net.InterfaceByName(iname)
		if err != nil {
			slog.Error("Failed to get interface for UDP listen", "interface", iname, "error", err)
			continue
		}
		ifaceAddrs, err := addrs.Addrs()
		if err != nil {
			slog.Error("Failed to get addresses for UDP listen", "interface", iname, "error", err)
			continue
		}
		for _, addr := range ifaceAddrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLoopback() {
				continue
			}
			for _, port := range cli.Port {
				laddr := &net.UDPAddr{IP: ip, Port: int(port)}
				conn, err := net.ListenUDP("udp", laddr)
				if err != nil {
					slog.Warn("Failed to listen on UDP", "interface", iname, "address", laddr.String(), "error", err)
					continue
				}
				wg.Add(1)
				go func(c *net.UDPConn, ifn string, p int) {
					defer wg.Done()
					defer c.Close()
					buf := make([]byte, 2048)
					for {
						select {
						case <-ctx.Done():
							return
						default:
							if err := c.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
								slog.Warn("Failed to set UDP read deadline", "interface", ifn, "port", p, "error", err)
								return
							}
							_, _, err := c.ReadFromUDP(buf)
							if err != nil {
								if ne, ok := err.(net.Error); ok && ne.Timeout() {
									continue
								}
								slog.Warn("UDP listen error", "interface", ifn, "port", p, "error", err)
								return
							}
							// Packet is read and ignored
						}
					}
				}(conn, iname, int(port))
			}
		}
	}
}
