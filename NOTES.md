# Misc Notes

 1. Just realized that 
	[according to the docs](https://godoc.org/golang.org/x/net/ipv4#RawConn.WriteTo),
	go only supports sendto() 
	on Linux/Darwin and not FreeBSD.  WTF?
 1. [FreeBSD supports sendto()](https://forums.freebsd.org/threads/solved-creating-and-transmitting-raw-ip-packets.44058/)
