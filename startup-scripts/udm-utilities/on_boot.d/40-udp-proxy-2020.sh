#!/bin/sh
# This file should be named: /mnt/data/on_boot.d/40-udp-proxy-2020.sh

CONTAINER=udp-proxy-2020  # name of the container
TAG=latest                # tag to use for docker image

. /mnt/data/udp-proxy-2020/udp-proxy-2020.conf

if [ -z "$PORTS" ]; then
    echo "Error: 'PORTS' is not set in config file" >/dev/stderr
    exit 1
fi

if [ -z "$INTEFACES" ]; then
    echo "Error: 'INTERFACES' is not set in config file" >/dev/stderr
    exit 1
fi

if [ -z "$TIMEOUT" ]; then
    TIMEOUT=250
fi

if [ -z "$CACHETTL" ]; then
    CACHETTL=90
fi


if podman container exists ${CONTAINER} ; then
    podman start ${CONTAINER}
else
    podman run -i -d --rm --net=host --name ${CONTAINER} \
        -e PORTS=${PORTS} -e INTERFACES=${INTERFACES} \
        -e TIMEOUT=${TIMEOUT} -e CACHETTL=${CACHETTL} \
        -e EXTRA_ARGS="${EXTRA_ARGS}" \
        synfinatic/udp-proxy-2020:${TAG}
fi
