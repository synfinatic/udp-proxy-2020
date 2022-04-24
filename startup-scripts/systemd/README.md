# systemd startup script

## Setup

 1. Copy systemd config file `udp-proxy-20202.service` to `/usr/lib/systemd/system/`
 1. Create udp-proxy-2020 config file: `/etc/udp-proxy-2020.conf`  (see below)
 1. Run `systemctl daemon-reload` to reload the systemctl configuration
 1. Run `systemctl enable udp-proxy-2020` to start the service automatically on boot
 1. Run `systemctl start udp-proxy-2020` to start the service
 1. Run `systemctl status udp-proxy-2020` to check that the service is running

## /etc/udp-proxy-2020.conf 

This file may have comments (lines which start with a `#`), but must have a line:

ARGS="*\<arguments passed to udp-proxy-2020\>*"

Typically it will look something like this:

`ARGS="--interface eth0,eth1 --port 9003"`

For a full list of possible arguments and their meaning, please run: `udp-proxy-2020 -h`
