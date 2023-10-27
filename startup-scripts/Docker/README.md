# Docker

## Configuration

Running `udp-proxy-2020` via a docker container is a little more complicated
than a typical docker container because you have to deal with two networks.

To make things work, you need to specify an IP address on the two (or more)
network interfaces you wish to bridge.  So if you have two networks:

 1. 192.168.5.0/24 on eth0
 1. 192.168.15.0/24 on eth1

You would need to find an available IP address on each of those networks and then
assign them to the `udp-proxy-2020` docker container.

Let's assume you choose __192.168.5.2__ and __192.168.15.2__, then you would use
the following [docker-compose.yaml](https://docs.docker.com/compose/compose-file/compose-file-v3/):

```yaml
version: '3.3'
services:
  udp-proxy-2020:
    container_name: udp-proxy-2020
    image: synfinatic/udp-proxy-2020:latest
    environment:
      - PORTS=9003
      - INTERFACES=eth0,eth1  # specify network interfaces here
      - EXTRA_ARGS=-L debug
      - TIMEOUT=250
      - CACHETTL=90
    restart: unless-stopped
    networks:
      macvlan_roon:
        ipv4_address: 192.168.5.2  # eth0 IP
      macvlan_main:
        ipv4_address: 192.168.15.2 # eth1 IP


# tell docker-compose that these networks already exist
networks:
    macvlan_roon:
      external: true
    macvlan_main:
      external: true
```

## Running

Just edit `docker-compose.yaml` and modify the environment variables as necessary
and then run `docker-compose up` to start!

