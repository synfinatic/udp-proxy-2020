# pfSense/OPNsense/BSD startup scripts

Note that these config files now support pfSense v2.5.0

## Configuration

 1. Add the line `udp_proxy_2020_enable=YES` to [/etc/rc.conf.local](etc/rc.conf.local)
    (file may need to be created)
 1. Create [/usr/local/etc/udp-proxy-2020.conf](usr/local/etc/udp-proxy-2020.conf)
    on your firewall and edit as necessary for your needs.
 1. Copy over [/usr/local/etc/rc.d/udp-proxy-2020](usr/local/etc/rc.d/udp-proxy-2020)
 1. Copy the correct [udp-proxy-2020 binary](
    https://github.com/synfinatic/udp-proxy-2020/releases) for your system to
    `/usr/local/bin/udp-proxy-2020` (yes, you have to rename the file!)
 1. Ensure that `/usr/local/bin/udp-proxy-2020` and
    `/usr/local/etc/rc.d/udp-proxy-2020` have the correct permissions by running:
    `chmod 755 /usr/local/etc/rc.d/udp-proxy-2020 /usr/local/bin/udp-proxy-2020`
 1. If you want the service to auto-start on boot, in the pfSense webui:
     1. If necessary, install the `shellcmd` package (`System -> Package Manager`)
     2. Navigate to: `Services -> Shellcmd`
     3. Click `Add` and fill out the form:
        * __Command__: `/usr/local/etc/rc.d/udp-proxy-2020 start`
        * __Shellcmd Type__: `shellcmd`
        * __Description__: `Start udp-proxy-2020 at boot`

## Run

Execute (as root) `service udp-proxy-2020 start`

## Other info

Things to keep in mind:

 * Tested to work with both Wiregard and OpenVPN on pfSense 2.6.0
 * You may need to ssh into your firewall and run `ifconfig` to get the name of
    the VPN interface

Additional commands:

 * Stop the service: `service udp-proxy-2020 stop` 
 * Check status of the service: `service udp-proxy-2020 status`


## OPNsense / FreeBSD

OPNsense and FreeBSD have a different way of running scripts at boot and you
should read their documentation for details:

 * [OPNsense](https://docs.opnsense.org/development/backend/autorun.html)
 * [FreeBSD](https://www.freebsd.org/cgi/man.cgi?query=rc.d&sektion=8&n=1)
