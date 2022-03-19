FROM golang:1.16-alpine as builder

ARG VERSION

# base applications
RUN apk add --update git build-base libpcap libpcap-dev && \
    mkdir udp-proxy-2020
COPY . udp-proxy-2020/

RUN cd udp-proxy-2020 && \
    DOCKER_VERSION=${VERSION} make .docker && \
    mkdir -p /usr/local/bin && \
    cp dist/udp-proxy-2020 /usr/local/bin/udp-proxy-2020

FROM alpine
RUN apk add --update libpcap && \
    mkdir -p /usr/local/bin
COPY --from=builder /usr/local/bin/udp-proxy-2020 /usr/local/bin/

ENV PORTS=9003
ENV INTERFACES=""
ENV TIMEOUT=250
ENV CACHETTL=90
ENV EXTRA_ARGS=""
CMD /usr/local/bin/udp-proxy-2020 --port $PORTS \
    --interface $INTERFACES --timeout $TIMEOUT \
    --cache-ttl $CACHETTL $EXTRA_ARGS
