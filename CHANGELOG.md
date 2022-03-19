# udp-proxy-2020 Changelog

## Unreleased

## v0.0.10 - 2022-03-18

Fixed:
 - Docker container would not start.

## v0.0.9 - 2022-02-21

Added:

 - Linux/ARMv5 support
 - Add support for writing pcap files for debugging #79

Changed:

 - Use Go 1.16
 - Simplify docker images for ARM builds 
 - --cache-ttl is now 3 hours
 - ARMv6/v7 now have unique binaries and use hardware floating point
 - No more "arm32" builds which isn't a real ARM arch
 - Remove str2pcap since we have direct pcap support now
 - Switch from pflag to Kong for CLI arg parsing
 - Update to logrus 1.8.1

## v0.0.8 - 2021-11-07

Changed:

 - Use FreeBSD 12.2 for building binaries (pfSense 2.5.x)
 - Add support for building FreeBSD ARM64 (aarch64), ARMv6 and ARMv7 binaries #53
 - FreeBSD AMD64 now has a static linked binary available
 - Update Makefile targets and improve `make help`
 - Release binaries no longer end in `-static`
 - Release binaries are now stripped and about 33% of their previous sizes
 - Improve debug output

Fixed: 

- Now use 'src net x.x.x.x/y' in the BPF filter to restrict packets we forward
    for platforms like Netgate SG5100 where pcap.SetDirection() doesn't work 
    to prevent infinite loops #71

## v0.0.7 - 2021-05-23

Added:

 - arm32el binary as part of official release
 - arm32hf binary as part of official release
 - systemd startup script & docs
 - Start signing releases
 - Add UDM Utilities startup scripts
 - Build ARM64 Docker container for UDM
 - Update Alpine/Go for Docker
 - Add --logfile option

Fixed:

 - Small tweaks to docs & makefile targets
 - pfSense/FreeBSD rc.d scripts now support FreeBSD 12.x/pfSense 2.5.0
 - Fix building on FreeBSD due to bash error

## v0.0.6 - 2021-01-18

Added:

- Support for cross-compiling arm64/Linux for RasPi/Ubiquiti UDM(Pro) #31
- Support Site-to-Site OpenVPN tunnels via --fixed-ip #41
- Add startup scripts for pfSense/BSD and link to tooling for Ubiquiti Dream
    Machine (Pro)
- Add docker image for running udp-proxy-2020 in a container
- Add docker-compose support

Fixed:

- Vagrant file for FreeBSD now always builds the latest code #39

## v0.0.5 - 2020-10-16

Added:

- Support for Wireguard (LinkType RAW) interfaces #29
- Add str2pcap for improved debugging of logs

## v0.0.4 - 2020-10-02

Initial release
