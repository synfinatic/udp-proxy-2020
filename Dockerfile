FROM alpine:latest
ENV PROJECT=udp-proxy-2020
ENV GOPATH=/go
RUN apk add go libpcap libpcap-dev make
COPY . /go/src/$PROJECT/
RUN cd /go/src/$PROJECT && make 
CMD cp /go/src/$PROJECT/dist/* /go/dist/ 
