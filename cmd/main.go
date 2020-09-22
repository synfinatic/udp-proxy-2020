package main

import (
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"os"
	"os/user"
	"sync"
)

func main() {
	var listen = []string{}
	var promisc = []string{}
	var ports = []int32{}
	var timeout int64
	var debug bool
	var ilist bool

	// option parsing
	flag.StringArrayVar(&listen, "listen", []string{}, "zero or more non-promisc interface@dstip")
	flag.StringArrayVar(&promisc, "promisc", []string{}, "zero or more promiscuous interface@dstip")
	flag.Int32SliceVar(&ports, "port", []int32{}, "one or more UDP ports to process")
	flag.Int64Var(&timeout, "timeout", 250, "timeout in ms")
	flag.BoolVar(&debug, "debug", false, "Enable debugging")
	flag.BoolVar(&ilist, "list-interfaces", false, "List available interfaces and exit")

	flag.Parse()

	// turn on debugging?
	if debug == true {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
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
	total := len(promisc) + len(listen)
	if total < 2 {
		log.Fatal("Please specify --promisc and --listen at least twice in total")
	}

	// handle our timeout & bpf filter for ports
	to := parseTimeout(timeout)
	bpf_filter := buildBPFFilter(ports)

	// init the listeners which are not promisc
	listeners := initalizeListeners(listen, false, bpf_filter, ports, to)
	// do the same for the promisc list which is
	for _, l := range initalizeListeners(promisc, true, bpf_filter, ports, to) {
		listeners = append(listeners, l)
	}

	// init each listener
	for _, l := range listeners {
		initalizeInterface(l)
		defer l.handle.Close()
	}

	// start handling packets
	var wg sync.WaitGroup
	spf := SendPktFeed{}
	log.Debug("Initialization complete!")
	for _, l := range listeners {
		wg.Add(1)
		go l.handlePackets(&spf, &wg)
	}
	wg.Wait()
}
