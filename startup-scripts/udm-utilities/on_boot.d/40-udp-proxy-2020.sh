#!/bin/sh
# This file should be named: /mnt/data/on_boot.d/40-udp-proxy-2020.sh

ARG1=${1:-run}

CONTAINER=udp-proxy-2020  # name of the container
TAG=latest                # tag to use for docker image

. /mnt/data/udp-proxy-2020/udp-proxy-2020.conf

if [ -z "$PORTS" ]; then
    echo "Error: 'PORTS' is not set in config file" >/dev/stderr
    exit 1
fi

if [ -z "$INTERFACES" ]; then
    echo "Error: 'INTERFACES' is not set in config file" >/dev/stderr
    exit 1
fi

if [ -z "$TIMEOUT" ]; then
    TIMEOUT=250
fi

if [ -z "$CACHETTL" ]; then
    CACHETTL=90
fi

if [ "$ARG1" = "clean" ]; then
    if podmain container exists ${CONTAINER}; then
        podman stop ${CONTAINER}
        podman container rm ${CONTAINER}
    fi
fi

if ! podman container exists ${CONTAINER} ; then
    podman create -i -d --net=host --name ${CONTAINER} \
        -e PORTS=${PORTS} -e INTERFACES=${INTERFACES} \
        -e TIMEOUT=${TIMEOUT} -e CACHETTL=${CACHETTL} \
        -e EXTRA_ARGS="${EXTRA_ARGS}" \
        --log-opt max-size=10mb \
        --restart always \
        synfinatic/udp-proxy-2020:${TAG}
fi
podman start ${CONTAINER}
