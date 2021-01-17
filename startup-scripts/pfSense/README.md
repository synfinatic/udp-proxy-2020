# pfSense/BSD startup scripts

## Configuration

 1. Edit `etc/local/etc/udp-proxy-2020.conf` as necessary
 1. Copy files to the directories on your pfSense/BSD box
 1. Edit `/etc/rc.conf.local` and add the line `udp_proxy_2020_enable=YES`
 1. Copy udp-proxy-2020 binary to `/usr/local/etc/bin/udp-proxy-2020`

## Run

Execute `/usr/local/etc/rc.d/udp-proxy-2020 start`
