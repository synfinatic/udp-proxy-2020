package main

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

var Version = "unknown"
var Buildinfos = "unknown"
var Tag = "NO-TAG"
var CommitID = "unknown"

func main() {
	var interfaces = []string{}
	var ports = []int32{}
	var timeout int64
	var cachettl int64
	var debug bool
	var version bool
	var ilist bool

	// option parsing
	flag.StringSliceVar(&interfaces, "interface", []string{}, "Two or more interfaces to use")
	flag.Int32SliceVar(&ports, "port", []int32{}, "One or more UDP ports to process")
	flag.Int64Var(&timeout, "timeout", 250, "Timeout in ms")
	flag.Int64Var(&cachettl, "cachettl", 90, "Client IP cache TTL in sec")
	flag.BoolVar(&debug, "debug", false, "Enable debugging")
	flag.BoolVar(&ilist, "list-interfaces", false, "List available interfaces and exit")
	flag.BoolVar(&version, "version", false, "Print version and exit")

	flag.Parse()

	// log.DisableLevelTruncation(true) <-- supposed to work, but doesn't?

	// turn on debugging?
	if debug == true {
		log.SetReportCaller(true)
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	if version == true {
		fmt.Printf("udp-proxy-2020 Version %s -- Copyright 2020 Aaron Turner\n", Version)
		fmt.Printf("%s (%s) built at %s\n", CommitID, Tag, Buildinfos)
		os.Exit(0)
	}

	if ilist == true {
		listInterfaces()
		os.Exit(0)
	}

	// Neeed at least two interfaces
	if len(interfaces) < 2 {
		log.Fatal("Please specify two or more interfaces via --interface")
	}

	// handle our timeout
	to := parseTimeout(timeout)

	// create our Listeners
	var seenInterfaces = []string{}
	var listeners = []Listen{}
	for _, iface := range interfaces {
		// check for duplicates
		if stringPrefixInSlice(iface, seenInterfaces) {
			log.Fatalf("Can't specify the same interface (%s) multiple times", iface)
		}
		seenInterfaces = append(seenInterfaces, iface)

		netif, err := net.InterfaceByName(iface)
		if err != nil {
			log.Fatalf("Unable to find interface: %s: %s", iface, err)
		}

		var promisc bool = (netif.Flags & net.FlagBroadcast) == 0
		listeners = append(listeners, newListener(netif, promisc, ports, to))
	}

	// init each listener
	ttl, _ := time.ParseDuration(fmt.Sprintf("%ds", cachettl))
	for i := range listeners {
		initializeInterface(&listeners[i])
		listeners[i].clientTTL = ttl
		defer listeners[i].handle.Close()
	}

	// start handling packets
	var wg sync.WaitGroup
	spf := SendPktFeed{}
	log.Debug("Initialization complete!")
	for i := range listeners {
		wg.Add(1)
		go listeners[i].handlePackets(&spf, &wg)
	}
	wg.Wait()
}
