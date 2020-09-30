FROM alpine:latest
ENV PROJECT=udp-proxy-2020
ENV GOPATH=/go
RUN apk add go libpcap libpcap-dev make
RUN mkdir /build
COPY . /build/$PROJECT/
RUN cd /build/$PROJECT && make 
