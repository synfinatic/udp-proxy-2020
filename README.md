# udp-proxy-2020

A crappy UDP proxy for the year 2020 and beyond.

## What is this for?

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
broadcasts which are then dropped on the floor.

## So what does this do?

Instead of using a normal UDP socket to listen for broadcast messages, udp-proxy-2020 
uses [libpcap](https://github.com/the-tcpdump-group/libpcap) to "sniff" the UDP 
broadcast messages.  This means it can be a lot more flexible about what packets
it "sees" so it can then forward them via a normal UDP socket.  If this makes
you go "ew", well, welcome to 2020.

## The good news...

I'm writing this in GoLang so at least cross compiling onto your random Linux/FreeBSD
router/firewall is reasonably easy.  No ugly cross-compling C or trying to install
Python/Ruby and a bunch of libraries.
