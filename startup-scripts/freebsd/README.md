# FreeBSD

FreeBSD, OpnSense, pfSense, etc users should now use the provided FreeBSD packages (`pkg add udp-proxy-2020...`).  The followling files will be automatically installed:

* `/usr/local/sbin/udp-proxy-2020` -- service binary
* `/usr/local/etc/udp-proxy-2020.conf.sample` -- sample config file
* `/usr/local/etc/rc.d/udp-proxy-2020` -- service script to start/stop udp-proxy-2020
* `/etc/rc.conf.d/udp_proxy_2020` -- service enable config file

For new installs, you will also need to copy the provided `udp-proxy-2020.conf.sample` to
`/usr/local/etc/udp-proxy-2020.conf` and edit for your system/needs.

Then you should be able to start the service via `service udp-proxy-2020 start` and verify it
is running with `service udp-proxy-2020 status`.

By default, `udp-proxy-2020` is configured to start on boot, so edit
`/etc/rc.conf.d/udp_proxy_2020` if you wish to disable that.

**Note:** if you get a segfault during installing the package, it is because one of the above files already exists on your system!