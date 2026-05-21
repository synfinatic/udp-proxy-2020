package proxy

import (
	"github.com/gopacket/gopacket/layers"
)

/*
 * These are not defined as layers.LinkType values, but ARE defined as
 * layers for decoding purposes: https://github.com/gopacket/gopacket/blob/master/layers/enums.go#L386
 *
 * But only one of these are actually valid at runtime based on your OS.
 * More context: https://www.tcpdump.org/linktypes.html
 *
 * Anyways, the RAW values per libpcap's pcap/dlt.h should be 12 or 14
 * based on your OS and nothing should have a value of 101.
 * https://github.com/gopacket/gopacket/blob/master/layers/enums.go#L105
 * No idea what the gopacket people were thinking.
 */
const (
	LinkTypeRawOpenBSD layers.LinkType = 14 // Raw DLT on OpenBSD
	LinkTypeRawOthers  layers.LinkType = 12 // Raw DLT everywhere else
)
