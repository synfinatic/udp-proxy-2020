# udp-proxy-2020 Changelog

## Unreleased

Added:

 - arm32el binary as part of official release
 - arm32hf binary as part of official release
 - systemd startup script & docs
 - Start signing releases
 - Add UDM Utilities startup scripts

Fixed:

 - Small tweaks to docs & makefile targets
 - pfSense/FreeBSD rc.d scripts now support FreeBSD 12.x/pfSense 2.5.0

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
