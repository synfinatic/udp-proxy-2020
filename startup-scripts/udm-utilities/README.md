# UDP-PROXY-2020

## Overview

[UDP Proxy 2020](https://github.com/synfinatic/udp-proxy-2020) forwards UDP
broadcast/multicast  messages across VLANs and VPN connections.  Useful for
[Roon](https://roonlabs.com) and apparently various computer games.

## Requirements

 1. You have successfully setup the on boot script described
    [here](https://github.com/boostchicken/udm-utilities/tree/master/on-boot-script)

## Customization

Edit [udp-proxy-2020.conf](udp-proxy-2020.conf) and modify the variables
according to your needs.

 1. Figure out what network interfaces (including VPN tunnel interfaces) and/or
    VLAN sub-interfaces you want to forward traffic on and set `INTERFACES`.
 1. Figure out what UDP ports you want forwarded and set `PORTS`.
 1. For more advanced configs, [read the udp-proxy-2020 docs](
    https://github.com/synfinatic/udp-proxy-2020/README.md)

Note that both `INTERFACES` and `PORTS` support multiple values separated by commas
while the `EXTRA_VARS` variable should be a single string in quotes.

## Steps

 1. Make a directory for the config file:

   ```sh
    mkdir -p /mnt/data/udp-proxy-2020
    ```

 1. Edit `udp-proxy-2020.conf` for your needs.
 1. Copy modified `udp-proxy-2020.conf` in `/mnt/data/udp-proxy-2020`
 1. Copy [40-udp-proxy-2020.sh](on_boot.d/40-udp-proxy-2020.sh) to
    `/mnt/data/on_boot.d`
 1. Execute `/mnt/data/on_boot.d/40-udp-proxy-2020.sh`
