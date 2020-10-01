package main

import (
	"fmt"
	"os"
	"os/user"
	"sync"

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
	var debug bool
	var version bool
	var ilist bool

	// option parsing

	flag.StringSliceVar(&interfaces, "interface", []string{}, "interfaces to use")
	flag.Int32SliceVar(&ports, "port", []int32{}, "one or more UDP ports to process")
	flag.Int64Var(&timeout, "timeout", 250, "timeout in ms")
	flag.BoolVar(&debug, "debug", false, "Enable debugging")
	flag.BoolVar(&ilist, "list-interfaces", false, "List available interfaces and exit")
	flag.BoolVar(&version, "version", false, "Print version and exit")

	flag.Parse()

	log.SetReportCaller(true)
	// log.DisableLevelTruncation(true)

	// turn on debugging?
	if debug == true {
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

	// make sure we're root
	if u, err := user.Current(); err != nil {
		log.Fatal(err)
	} else if u.Uid != "0" {
		log.Fatal("need to run as root in order for raw sockets to work")
	}

	// Neeed at least two interfaces
	if len(interfaces) < 2 {
		log.Fatal("Please specify --interfaces at least twice")
	}

	// handle our timeout & bpf filter for ports
	to := parseTimeout(timeout)
	bpf_filter := buildBPFFilter(ports)

	// init the listeners
	listeners := initializeListeners(interfaces, bpf_filter, ports, to)

	// init each listener
	for i := range listeners {
		initializeInterface(&listeners[i])
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
