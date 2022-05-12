# pfSense/BSD startup scripts

Note that these config files now support pfSense v2.5.0

## Configuration

 1. Create [/usr/local/etc/udp-proxy-2020.conf](usr/local/etc/udp-proxy-2020.conf)
    on your firewall and edit as necessary for your needs.
 1. Add the line `udp_proxy_2020_enable=YES` to `/etc/rc.conf.local` (file may need to be created)
 1. Copy over [/usr/local/etc/rc.d/udp-proxy-2020](usr/local/etc/rc.d/udp-proxy-2020)
 1. Copy the correct [udp-proxy-2020 binary](
    https://github.com/synfinatic/udp-proxy-2020/releases) for your system to 
    `/usr/local/bin/udp-proxy-2020` (yes, you have to rename the file!)
 1. Ensure that `/usr/local/bin/udp-proxy-2020` and 
    `/usr/local/etc/rc.d/udp-proxy-2020` have the correct permissions by running:
    `chmod 755 /usr/local/etc/rc.d/udp-proxy-2020 /usr/local/bin/udp-proxy-2020`

## Run

Execute (as root) `service udp-proxy-2020 start`

## Other info

Things to keep in mind:

 * Tested to work with both Wiregard and OpenVPN on pfSense 2.6.0
 * You may need to ssh into your firewall and run `ifconfig` to get the name of the VPN interface

Additional commands:

 * Stop the service: `service udp-proxy-2020 stop`  
 * Check status of the service: `service udp-proxy-2020 status`
