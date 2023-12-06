package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	log "github.com/phuslu/log"
)

var (
	Version    = "unknown"
	Buildinfos = "unknown"
	Tag        = "NO-TAG"
	CommitID   = "unknown"
	Delta      = ""
)

type CLI struct {
	Interface      []string `kong:"short='i',help='Two or more interfaces to use'"`
	FixedIp        []string `kong:"short='I',help='IPs to always send to iface@ip'"`
	Port           []int32  `kong:"short='p',help='One or more UDP ports to process'"`
	Timeout        int64    `kong:"short='t',default=250,help='Timeout in msec'"`
	CacheTTL       int64    `kong:"short='T',default=180,help='Client IP cache TTL in minutes'"`
	DeliverLocal   bool     `kong:"short='l',help='Deliver packets locally over loopback'"`
	Level          string   `kong:"short='L',default='info',enum='trace,debug,info,warn,error',help='Log level [trace|debug|info|warn|error]'"`
	LogLines       bool     `kong:"help='Print line number in logs'"`
	Logfile        string   `kong:"default='stderr',help='Write logs to filename'"`
	Pcap           bool     `kong:"short='P',help='Generate pcap files for debugging'"`
	PcapPath       string   `kong:"short='d',default='/root',help='Directory to write debug pcap files'"`
	ListInterfaces bool     `kong:"help='List available interfaces and exit'"`
	Version        bool     `kong:"short='v',help='Print version information'"`
	NoListen       bool     `kong:"help='Do not actively listen on UDP port(s)'"`
}

func main() {
	log.DefaultLogger = log.Logger{
		Level:  log.InfoLevel,
		Caller: 0,
		Writer: &log.IOWriter{os.Stderr},
	}

	cli := parseArgs()

	// handle our timeout
	timeout := parseTimeout(cli.Timeout)

	fixed_ip := map[string][]string{}
	for _, fip := range cli.FixedIp {
		split := strings.Split(fip, "@")
		if len(split) != 2 {
			log.Fatal().Msgf("--fixed-ip %s is not in the correct format of <interface>@<ip>", fip)
		}
		if net.ParseIP(split[1]) == nil {
			log.Fatal().Msgf("--fixed-ip %s IP address is not a valid IPv4 address", fip)
		}
		if !stringInSlice(split[0], cli.Interface) {
			log.Fatal().Msgf("--fixed-ip %s interface must be specified via --interface", fip)
		}
		fixed_ip[split[0]] = append(fixed_ip[split[0]], split[1])
	}

	// create our Listeners
	seenInterfaces := []string{}
	listeners := []Listen{}
	for _, iface := range cli.Interface {
		// check for duplicates
		if stringPrefixInSlice(iface, seenInterfaces) {
			log.Fatal().Msgf("Can't specify the same interface (%s) multiple times", iface)
		}
		seenInterfaces = append(seenInterfaces, iface)

		netif, err := net.InterfaceByName(iface)
		if err != nil {
			log.Fatal().Err(err).Msgf("Unable to find interface: %s", iface)
		}

		promisc := (netif.Flags & net.FlagBroadcast) == 0
		l := newListener(netif, promisc, false, cli.Port, timeout, fixed_ip[iface])
		listeners = append(listeners, l)
	}

	if cli.DeliverLocal {
		// Create loopback listener
		netif, err := net.InterfaceByName(getLoopback())
		if err != nil {
			log.Fatal().Err(err).Msg("unable to find loopback interface")
		}

		l := newListener(netif, false, true, cli.Port, timeout, []string{"127.0.0.1"})
		listeners = append(listeners, l)
	}

	// init each listener
	ttl, _ := time.ParseDuration(fmt.Sprintf("%dm", cli.CacheTTL))
	for i := range listeners {
		initializeInterface(&listeners[i])
		if cli.Pcap {
			if fName, err := listeners[i].OpenWriter(cli.PcapPath, In); err != nil {
				log.Fatal().Err(err).Str("pcap file", fName).Msg("unable to open")
			}
			if fName, err := listeners[i].OpenWriter(cli.PcapPath, Out); err != nil {
				log.Fatal().Err(err).Str("pcap file", fName).Msg("unable to open")
			}
			if fName, err := listeners[i].OpenWriter(cli.PcapPath, InOut); err != nil {
				log.Fatal().Err(err).Str("pcap file", fName).Msg("unable to open")
			}
		}
		listeners[i].clientTTL = ttl
		defer listeners[i].handle.Close()
	}

	// Sink broadcast messages
	if !cli.NoListen {
		for _, l := range listeners {
			if err := l.SinkUdpPackets(); err != nil {
				log.Fatal().Err(err).Msg("unable to init SinkUdpPackets")
			}
		}
	}

	// start handling packets
	var wg sync.WaitGroup
	spf := SendPktFeed{}
	log.Debug().Msg("initialization complete!")
	for i := range listeners {
		wg.Add(1)
		go listeners[i].handlePackets(&spf, &wg)
	}
	wg.Wait()
}

func parseArgs() CLI {
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
	var logLevel log.Level
	switch cli.Level {
	case "trace":
		logLevel = log.TraceLevel
	case "debug":
		logLevel = log.DebugLevel
	case "warn":
		logLevel = log.WarnLevel
	case "info":
		logLevel = log.InfoLevel
	case "error":
		logLevel = log.ErrorLevel
	}

	caller := 0
	if cli.LogLines {
		caller = 1
	}

	if cli.ListInterfaces {
		listInterfaces()
		os.Exit(0)
	}

	if len(cli.Interface) < 2 {
		log.Fatal().Msg("please specify two or more --interface")
	}
	if len(cli.Port) < 1 {
		log.Fatal().Msg("please specify one or more --port")
	}

	var writer log.Writer
	switch {
	case cli.Logfile != "stderr":
		writer = &log.FileWriter{
			Filename:     cli.Logfile,
			MaxSize:      500 * 1024 * 1024,
			FileMode:     0600,
			MaxBackups:   7,
			EnsureFolder: true,
			LocalTime:    true,
			Cleaner: func(filename string, _ int, matches []os.FileInfo) {
				dir := filepath.Dir(filename)
				var total int64
				for i := len(matches) - 1; i >= 0; i-- {
					total += matches[i].Size()
					if total > 5*1024*1024*1024 {
						os.Remove(filepath.Join(dir, matches[i].Name()))
					}
				}
			},
		}
	default:
		writer = &log.ConsoleWriter{
			ColorOutput:    true,
			QuoteString:    true,
			EndWithMessage: true,
		}
	}

	log.DefaultLogger = log.Logger{
		Level:      logLevel,
		Caller:     caller,
		TimeField:  "date",
		TimeFormat: "2006-01-02",
		Writer:     writer,
	}

	return cli
}
