module github.com/synfinatic/udp-proxy-2020

go 1.24.7

require (
	github.com/alecthomas/kong v1.6.1
	github.com/davecgh/go-spew v1.1.1
	github.com/gopacket/gopacket v1.4.0
	github.com/sirupsen/logrus v1.9.3
	golang.org/x/net v0.39.0 // indirect; security
)

require github.com/ccoveille/go-safecast v1.6.1

require golang.org/x/sys v0.32.0 // indirect

// see: https://github.com/sirupsen/logrus/issues/1275
// require golang.org/x/sys v0.0.0-20210817190340-bfb29a6856f2 // indirect
