# udp-proxy-2020

A crappy UDP proxy for the year 2020 and beyond.

## About

### What is this for?

So I'm playing with [Roon](https://roonlabs.com) and I've got this complicated
home network that throws Roon for a loop.  I started debugging things and it
turns out Roon sends _broadcast_ messages to UDP/9003.  My firewall/router will
not forward these messages of course, because that's the right thing to do.

Unfortunately, I really want these broadcast messages to be forwarded to other
VLAN/subnets on my local network.  I started using
[udp-proxy-relay-redux](https://github.com/udp-redux/udp-broadcast-relay-redux)
which worked great at first.

But I also really like these messages forwarded over my OpenVPN connections
which utilize the `tun` driver which is a point-to-point interface and
_explicity_ does not support broadcasts.  This didn't work well with
udp-proxy-relay-redux because Roon is poorly behaved and still tries sending
"broadcasts" to the .255 address which are then dropped on the floor because my
VPN server does not have the address x.x.x.255.  Basically, on a point-to-point
interface, these "broadcasts" were being treated as a packet destined to another
host and rightfully ignored.

### So what does this do?

Instead of using a normal UDP socket to listen for broadcast messages, `udp-proxy-2020`
uses [libpcap](https://github.com/the-tcpdump-group/libpcap) to "sniff" the UDP
broadcast messages.  This means it can be a lot more flexible about what packets
it "sees" so it can then sends them via libpcap/packet injection out all the other
configured interfaces.  If this makes you go "ew", well,
[welcome to 2020](https://google.com/search?q=why+is+2020+the+worst).

### The good news...

I'm writing this in GoLang so at least cross compiling onto your random Linux/FreeBSD
router/firewall is reasonably easy.  No ugly cross-compling C or trying to install
Python/Ruby and a bunch of libraries.

*Also: HAHAHAHAHAHAHA!  None of that is true!*  Needing to use
[libpcap](https://www.tcpdump.org) means I have to cross compile using CGO because
[gopacket/pcapgo](https://gowalker.org/github.com/google/gopacket/pcapgo) only
supports Linux for reading & writing to (ethernet?) network interfaces.

## Supported Systems

Pretty much any Unix-like system is supported because the dependcy list is only
`libpcap` and `golang`.  I develop on MacOS and specifically target
[pfSense](https://www.pfsense.org)/FreeBSD and
[Ubiquiti](https://www.ui.com) USG/EdgeRouter as those are quite common among
the Roon user community.

I [release binaries](https://github.com/synfinatic/udp-proxy-2020/releases)
for Linux/x86_64, Linux/MIPS64, Linux/ARM64, FreeBSD/amd64 and MacOS/x86_64.

## Building udp-proxy-2020

If you are building for the same platform you intend to run `udp-proxy-2020`
then you just need to make sure you have `libpcap` and the necessary headers
(you may need a `-dev` package for that) and run `make` or `gmake` as
appropriate (we need GNU Make, not BSD Make).

If you need to build cross platform, then one of the following targets may help
you:

 * Linux on x86_64 `make linux-amd64` via [Docker](https://www.docker.com)
 * Linux on MIPS64 `make linux-mips64` (Linux/MIPS64 big-endian for Ubiquiti
    USG/EdgeRouter) via Docker
 * Linux on ARM64 `make linux-arm64` (Linux/ARM64 for Ubiquiti UDM/UDM Pro)
    via Docker
 * FreeBSD 11.3 on x86_64 `make freebsd` (pfSense 2.4) via
[Vagrant](https://www.vagrantup.com) & [VirtualBox](https://www.virtualbox.org)

You can get a full list of make targets and basic info about them by running:
`make help`.

## Usage

`udp-proxy-2020` is still under heavy development.  Run `udp-proxy-2020 --help`
for a current list of command line options.  Also, please note on many operating
systems you will need to run it as the `root` user.  Linux systems can
optionally grant the `CAP_NET_RAW` capability.

Currently there are only a few flags you probaly need to worry about:

 * `--interface` -- specify two or more network interfaces to listen on
 * `--port` -- specify one or more UDP ports to monitor
 * `--debug` -- enable debugging output
 * `--fixed-ip` -- Hardcode an <interface>@<ipaddr> to always send traffic to.  Useful
	for things like OpenVPN in site-to-site mode.

There are other flags of course, run `./udp-proxy-2020 --help` for a full list.

## Installation & Startup Scripts

There are now instructions and startup scripts available in the [startup-scripts](
startup-scripts) directory.  If you figure out how to add support for another
platform, please send me a pull request!

## FAQ

### So is it a "proxy"?  Are there any proxy config settings I need to configure in my app?

Nope, it's not a proxy.  It's more like a router.  You don't need to make
any changes other than running it on your home router/firewall.

### Then why did you call it udp-proxy-2020?

Honestly, I didn't really think much about the name and this was the first thing
that came to my mind.

### What network interface types are supported?

 * Ethernet
 * WiFi interfaces which appear as Ethernet
 * `tun` interfaces, like those used by [OpenVPN](https://openvpn.net)
 * `raw` interfaces, like those used by [Wireguard](https://www.wireguard.com)

### How can I get udp-proxy-2020 working with Wireguard on Ubiquiti USG?

So I haven't done this myself, but Bart Verhoeven over on the Roon Community
forums wrote up
[this really detailed how to](https://community.roonlabs.com/t/how-to-roon-mobile-over-wireguard-on-a-unifi-usg/124477).
