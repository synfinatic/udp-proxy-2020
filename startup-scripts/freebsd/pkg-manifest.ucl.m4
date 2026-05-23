name = "udp-proxy-2020";
version = "M4_VERSION";
origin = "net/udp-proxy-2020";
comment = "UDP Proxy 2020";
desc = "UDP proxy for forwarding UDP packets across network interfaces";
maintainer = "synfinatic@gmail.com";
www = "https://github.com/synfinatic/udp-proxy-2020";
prefix = "/usr/local";
arch = "M4_ABI";
licenses = [GPLv3];
categories = [net];
files {
    /usr/local/sbin/udp-proxy-2020 = "0755";
    /usr/local/etc/rc.d/udp-proxy-2020 = "0755";
    /usr/local/etc/udp-proxy-2020.conf.sample = "0644";
    /etc/rc.conf.d/udp_proxy_2020 = "0644";
}