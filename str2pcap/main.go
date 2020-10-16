package main

// Code to take a file with each line representing a packet in hex
// Intended to be used with the log output from udp-proxy-2020

import (
	"bufio"
	"encoding/hex"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

func main() {
	var out = flag.String("out", "", "Pcap file to create")
	var in = flag.String("in", "", "Input file name with packet data to read")
	var dlt = flag.Uint8("dlt", 1, "DLT value")
	var debug = flag.Bool("debug", false, "Enable debugging")

	flag.Parse()
	if *debug == true {
		log.SetReportCaller(true)
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	if len(*out) == 0 || len(*in) == 0 {
		log.Fatal("Please specify --in, --out and --dlt")
	}

	infile, err := os.Open(*in)
	if err != nil {
		log.Fatalf("--in %s: %s", *in, err)
	}
	inScanner := bufio.NewScanner(infile)
	inScanner.Split(bufio.ScanLines)

	fh, err := os.Create(*out)
	if err != nil {
		log.Fatalf("--out %s: %s", *out, err)
	}

	var linktype = layers.LinkType(*dlt)
	pcap := pcapgo.NewWriterNanos(fh)
	pcap.WriteFileHeader(65535, linktype)
	var i = 0
	for inScanner.Scan() {
		i += 1
		bytes, err := hex.DecodeString(inScanner.Text())
		if err != nil {
			log.Fatalf("reading line %d: %s", i, err)
		}

		ci := gopacket.CaptureInfo{
			Timestamp:      time.Time{},
			CaptureLength:  len(bytes),
			Length:         len(bytes),
			InterfaceIndex: 0,
		}
		err = pcap.WritePacket(ci, bytes)
		if err != nil {
			log.Fatal(err)
		}
	}

	infile.Close()
	// no method to close a gopcap Writer???? WTF?
}
