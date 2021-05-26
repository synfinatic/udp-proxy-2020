# udp-proxy-2020

A crappy UDP router for the year 2020 and beyond.

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
[Ubiquiti](https://www.ui.com) USG, EdgeRouter and DreamMachine/Pro as those
are quite common among the Roon user community.

I [release binaries](https://github.com/synfinatic/udp-proxy-2020/releases)
for Linux/x86\_64, ARM64, ARM32 & MIPS64, FreeBSD/x86\_64 and MacOS/x86\_64.

I hope to support FreeBSD/ARM64 [in the future](https://github.com/synfinatic/udp-proxy-2020/issues/53).

There is also a [docker image available](
https://hub.docker.com/repository/docker/synfinatic/udp-proxy-2020) for Linux on
amd64 and arm64 (like the Ubiquiti UDM).

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
 * Docker `make docker`

You can get a full list of make targets and basic info about them by running:
`make help`.

## Usage

Run `udp-proxy-2020 --help` for a current list of command line options.  
Also, please note on many operating systems you will need to run it as the 
`root` user.  Linux systems can optionally grant the `CAP_NET_RAW` capability.

Currently there are only a few flags you probaly need to worry about:

 * `--interface` -- Specify two or more network interfaces to listen on.
 * `--port` -- Specify one or more UDP ports to monitor.
 * `--debug` -- Enable debugging output.

Advanced options:

 * `--fixed-ip` -- Hardcode an <interface>@<ipaddr> to always send traffic to.
    Useful for things like OpenVPN in site-to-site mode.
 * `--timeout` -- Number of ms for pcap timeout value. (default is 250ms)
 * `--cachettl` -- Number of seconds to cache IPs for. (default is 90sec)
    This value may need to be increased if you have problems passing traffic to
    clients on OpenVPN tunnels if you can't use `--fixed-ip` because clients
    don't have a fixed ip.

There are other flags of course, run `./udp-proxy-2020 --help` for a full list.

Example:

```sh
udp-proxy-2020 --port 9003 --interface eth0,eth0.100,eth1,tun0 --cachettl 300
```

Would forward udp/9003 packets on four interfaces: eth0, eth1, VLAN100 on eth0 and tun0.
Client IP's on tun0 would be remembered for 5 minutes once they are learned.

Note: "learning" requires the client to send a udp/9003 message first!  If
your application requires a message to be sent *to* the client first, then you
would need to specify `--fixed-ip=1.2.3.4@tun0` where `1.2.3.4` is the IP address
of the client on tun0.

## Installation & Startup Scripts

There are now instructions and startup scripts available in the [startup-scripts](
startup-scripts) directory.  If you figure out how to add support for another
platform, please send me a pull request!

## FAQ

### Where can I download precompiled binaries?

From the [releases page](https://github.com/synfinatic/udp-proxy-2020/releases) on Github.

### So is it a "proxy"?  Are there any proxy config settings I need to configure in my app?

Nope, it's not a proxy.  It's more like a router.  You don't need to make
any changes other than running it on your home router/firewall.

### Then why did you call it udp-proxy-2020?

Honestly, I didn't really think much about the name and this was the first thing
that came to my mind.  Also, [naming is hard](https://martinfowler.com/bliki/TwoHardThings.html).

### What network interface types are supported?

 * Ethernet
 * WiFi interfaces which appear as Ethernet
 * `tun` interfaces, like those used by [OpenVPN](https://openvpn.net)
 * `raw` interfaces, like those used by [Wireguard](https://www.wireguard.com)
 * `vti` interfaces for site-to-site IPSec

Note that L2TP VPN tunnels on Linux are not compatible with udp-proxy-2020
because the Linux kernel exposes those interfaces as [Linux SLL](
https://wiki.wireshark.org/SLL) which does not provide an accurate decode
of the packets.

### How can I get udp-proxy-2020 working with Wireguard on Ubiquiti USG?

So I haven't done this myself, but Bart Verhoeven over on the Roon Community
forums wrote up
[this really detailed how to](https://community.roonlabs.com/t/how-to-roon-mobile-over-wireguard-on-a-unifi-usg/124477).

### What binary is right for me?

udp-proxy-2020 is built for multiple OS and hardware platforms:

 * MacOS/Intel x86_64: darwin-x86_64
 * Linux/Intel x86_64: linux-amd64
 * Linux/ARM64: linux-arm64
 * Linux/ARM32EL: linux-arm32
 * Linux/ARM32HF (hardware floating point): linux-arm32hf
 * Linux/MIPS64: linux-mips64
 * FreeBSD/Intel x86_64:: freebsd-amd64 (works with pfSense on x86)
