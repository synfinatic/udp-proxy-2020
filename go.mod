module github.com/synfinatic/udp-proxy-2020

go 1.26

toolchain go1.26.3

require (
	github.com/alecthomas/kong v1.15.0
	github.com/gopacket/gopacket v1.7.0
	golang.org/x/net v0.55.0 // indirect; security
)

require golang.org/x/sys v0.45.0 // indirect

// see: https://github.com/sirupsen/logrus/issues/1275
// require golang.org/x/sys v0.0.0-20210817190340-bfb29a6856f2 // indirect
