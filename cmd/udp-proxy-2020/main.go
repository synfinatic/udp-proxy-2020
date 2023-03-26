package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	log "github.com/sirupsen/logrus"
)

var Version = "unknown"
var Buildinfos = "unknown"
var Tag = "NO-TAG"
var CommitID = "unknown"
var Delta = ""

type CLI struct {
	Interface      []string `kong:"short='i',help='Two or more interfaces to use'"`
	FixedIp        []string `kong:"short='I',help='IPs to always send to iface@ip'"`
	Port           []int32  `kong:"short='p',help='One or more UDP ports to process'"`
	Timeout        int64    `kong:"short='t',default=250,help='Timeout in msec'"`
	CacheTTL       int64    `kong:"short='T',default=180,help='Client IP cache TTL in minutes'"`
	Level          string   `kong:"short='L',default='info',enum='trace,debug,info,warn,error',help='Log level [trace|debug|info|warn|error]'"`
	LogLines       bool     `kong:"help='Print line number in logs'"`
	Logfile        string   `kong:"default='stderr',help='Write logs to filename'"`
	Pcap           bool     `kong:"short='P',help='Generate pcap files for debugging'"`
	PcapPath       string   `kong:"short='d',default='/root',help='Directory to write debug pcap files'"`
	ListInterfaces bool     `kong:"short='l',help='List available interfaces and exit'"`
	Version        bool     `kong:"short='v',help='Print version information'"`
	NoListen       bool     `kong:"help='Do not actively listen on UDP port(s)'"`
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableLevelTruncation: true,
		PadLevelText:           true,
		DisableTimestamp:       true,
	})
	log.SetOutput(os.Stderr)
}

func main() {
	cli := CLI{}
	parser := kong.Must(
		&cli,
		kong.Name("udp-proxy-2020"),
		kong.Description("A crappy UDP proxy for the year 2020 and beyond!"),
		kong.UsageOnError(),
	)
	_, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)

	if cli.Version {
		delta := ""
		if len(Delta) > 0 {
			delta = fmt.Sprintf(" [%s delta]", Delta)
			Tag = "Unknown"
		}
		fmt.Printf("udp-proxy-2020 Version %s -- Copyright 2020-2022 Aaron Turner\n", Version)
		fmt.Printf("%s (%s)%s built at %s\n", CommitID, Tag, delta, Buildinfos)
		os.Exit(0)
	}

	// Setup Logging
	switch cli.Level {
	case "trace":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	}

	if cli.LogLines {
		log.SetReportCaller(true)
	}

	if cli.ListInterfaces {
		listInterfaces()
		os.Exit(0)
	}

	if len(cli.Interface) < 2 {
		log.Fatalf("Please specify two or more --interface")
	}
	if len(cli.Port) < 1 {
		log.Fatalf("Please specify one or more --port")
	}

	if cli.Logfile != "stderr" {
		file, err := os.OpenFile(cli.Logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			log.WithError(err).Fatalf("Unable to open log file: %s", cli.Logfile)
		}
		log.SetOutput(file)
	}

	// handle our timeout
	to := parseTimeout(cli.Timeout)

	var fixed_ip = map[string][]string{}
	for _, fip := range cli.FixedIp {
		split := strings.Split(fip, "@")
		if len(split) != 2 {
			log.Fatalf("--fixed-ip %s is not in the correct format of <interface>@<ip>", fip)
		}
		if net.ParseIP(split[1]) == nil {
			log.Fatalf("--fixed-ip %s IP address is not a valid IPv4 address", fip)
		}
		if !stringInSlice(split[0], cli.Interface) {
			log.Fatalf("--fixed-ip %s interface must be specified via --interface", fip)
		}
		fixed_ip[split[0]] = append(fixed_ip[split[0]], split[1])
	}

	// create our Listeners
	var seenInterfaces = []string{}
	var listeners = []Listen{}
	for _, iface := range cli.Interface {
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
		l := newListener(netif, promisc, cli.Port, to, fixed_ip[iface])
		listeners = append(listeners, l)
	}

	// init each listener
	ttl, _ := time.ParseDuration(fmt.Sprintf("%dm", cli.CacheTTL))
	for i := range listeners {
		initializeInterface(&listeners[i])
		if cli.Pcap {
			if fName, err := listeners[i].OpenWriter(cli.PcapPath, In); err != nil {
				log.Fatalf("Unable to open pcap file %s: %s", fName, err.Error())
			}
			if fName, err := listeners[i].OpenWriter(cli.PcapPath, Out); err != nil {
				log.Fatalf("Unable to open pcap file %s: %s", fName, err.Error())
			}
			if fName, err := listeners[i].OpenWriter(cli.PcapPath, InOut); err != nil {
				log.Fatalf("Unable to open pcap file %s: %s", fName, err.Error())
			}
		}
		listeners[i].clientTTL = ttl
		defer listeners[i].handle.Close()
	}

	// Sink broadcast messages
	if !cli.NoListen {
		for _, l := range listeners {
			if err := l.SinkUdpPackets(); err != nil {
				log.WithError(err).Fatalf("Unable to init SinkUdpPackets")
			}
		}
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
