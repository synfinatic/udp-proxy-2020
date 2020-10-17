package main

import (
	"runtime"

	"github.com/google/gopacket/layers"
)

/*
 * These are not defined as layers.LinkType values, but ARE defined as
 * layers for decoding purposes: https://github.com/google/gopacket/blob/master/layers/enums.go#L378
 *
 * But only one of these are actually valid at runtime based on your OS.
 * More context: https://www.tcpdump.org/linktypes.html
 *
 * Anyways, the RAW values per libpcap's pcap/dlt.h should be 12 or 14
 * based on your OS and nothing should have a value of 101.  No idea what
 * the gopacket people were thinking.
 */
const (
	LinkTypeRawOpenBSD layers.LinkType = 14 // Raw DLT on OpenBSD
	LinkTypeRawOthers  layers.LinkType = 12 // Raw DLT everywhere else
)

// Returns if the provided layertype is valid
func isValidLayerType(layertype layers.LinkType) bool {
	/*
	* List of Data Link Types (DLT) we support in sendPacket()
	* not to be confused with LINKTYPE_ values which are mostly, but not always
	* compatible: https://www.tcpdump.org/linktypes.html
	*
	* Specifically, DLT_RAW is 12 or 14 depending on your OS, but
	* LinkTypeRaw is 101 which causes lots of problems. :-/
	 */
	var validLinkTypes = []layers.LinkType{
		layers.LinkTypeLoop,
		layers.LinkTypeEthernet,
		layers.LinkTypeNull,
		layers.LinkTypeRaw,
	}

	// Look for standardized values
	for _, b := range validLinkTypes {
		if layertype == b {
			return true
		}
	}

	// Look for Raw value based on our running OS due to above mentioned issue
	if runtime.GOOS == "openbsd" && layertype == LinkTypeRawOpenBSD {
		return true
	}
	if runtime.GOOS != "openbsd" && layertype == LinkTypeRawOthers {
		return true
	}

	return false
}
