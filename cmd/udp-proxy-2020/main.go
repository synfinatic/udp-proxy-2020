package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gopacket/gopacket/pcapgo"
	log "github.com/sirupsen/logrus"
	"github.com/synfinatic/udp-proxy-2020/internal/config"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy/stages"
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

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableLevelTruncation: true,
		PadLevelText:           true,
		DisableTimestamp:       true,
	})
	log.SetOutput(os.Stderr)
}

func main() {
	cli := parseArgs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	timeout := config.ParseTimeout(cli.Timeout)
	ttl, _ := time.ParseDuration(fmt.Sprintf("%dm", cli.CacheTTL))

	dm, err := proxy.NewDeviceManager()
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize device manager")
	}

	if cli.ListInterfaces {
		dm.ListInterfaces()
		os.Exit(0)
	}

	bus := proxy.NewPacketBus()

	var pipelines []*proxy.Pipeline
	var transmitters []*stages.TransmitterSink

	interfaces := cli.Interface
	if cli.DeliverLocal {
		lb := dm.GetLoopback()
		if lb == "" {
			log.Fatal("Unable to find loopback interface")
		}
		interfaces = append(interfaces, lb)
	}

	// Group fixed IPs by interface
	fixedIPs := make(map[string][]string)
	for _, f := range cli.FixedIp {
		parts := strings.Split(f, "@")
		if len(parts) == 2 {
			fixedIPs[parts[0]] = append(fixedIPs[parts[0]], parts[1])
		}
	}

	for _, iname := range interfaces {
		netif, err := net.InterfaceByName(iname)
		if err != nil {
			log.WithError(err).Fatalf("Interface %s not found", iname)
		}

		handle, err := dm.CreateHandle(iname, (netif.Flags&net.FlagBroadcast) == 0, timeout)
		if err != nil {
			log.WithError(err).Fatalf("Failed to open %s", iname)
		}

		// Set BPF filter
		addrs, _ := dm.GetAddresses(iname)
		filter := config.BuildBPFFilter(cli.Port, addrs)
		if err := handle.SetBPFFilter(filter); err != nil {
			log.WithError(err).Fatalf("Failed to set BPF filter on %s", iname)
		}

		registry := stages.NewRegistryProcessor(ttl, fixedIPs[iname])

		pipeline := proxy.NewPipeline(stages.NewPcapSource(handle, iname))
		pipeline.AddProcessor(&stages.FilterProcessor{Iname: iname})
		pipeline.AddProcessor(registry)

		// Pcap file logging
		if cli.Pcap {
			fPath := filepath.Join(cli.PcapPath, fmt.Sprintf("udp-proxy-in-%s.pcap", iname))
			f, err := os.Create(fPath)
			if err == nil {
				w := pcapgo.NewWriter(f)
				w.WriteFileHeader(65536, handle.LinkType())
				pipeline.AddSink(&stages.PcapFileSink{Writer: w})
			}
		}

		// Forwarding to bus
		pipeline.AddSink(&stages.ForwardingSink{
			Feed:     bus,
			Iname:    iname,
			LinkType: handle.LinkType(),
		})

		// Broadcaster for this interface
		busChan := make(chan proxy.BusMessage, 100)
		bus.Subscribe(iname, busChan)

		// Broadcast address discovery
		var bcast net.IP
		pcapAddrs, _ := dm.GetAddresses(iname)
		for _, addr := range pcapAddrs {
			if addr.IP.To4() != nil && addr.Broadaddr != nil {
				bcast = addr.Broadaddr
				break
			}
		}
		if cli.DeliverLocal && iname == dm.GetLoopback() {
			bcast = net.ParseIP("127.0.0.1")
		}

		transmitter := &stages.TransmitterSink{
			Handle:           handle,
			Iname:            iname,
			HardwareAddr:     netif.HardwareAddr,
			Promisc:          (netif.Flags & net.FlagBroadcast) == 0,
			BroadcastAddress: bcast,
			PacketBus:        busChan,
			Registry:         registry,
		}

		pipelines = append(pipelines, pipeline)
		transmitters = append(transmitters, transmitter)
	}

	var wg sync.WaitGroup
	for _, p := range pipelines {
		wg.Add(1)
		go func(pipe *proxy.Pipeline) {
			defer wg.Done()
			if err := pipe.Run(ctx); err != nil {
				log.Errorf("Pipeline error: %v", err)
			}
		}(p)
	}

	for _, t := range transmitters {
		go t.Run()
	}

	log.Info("All pipelines started")
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
		fmt.Printf("udp-proxy-2020 Version %s -- Copyright 2020-2026 Aaron Turner\n", Version)
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

	if len(cli.Interface) < 2 && !cli.ListInterfaces && !cli.Version {
		log.Fatalf("Please specify two or more --interface")
	}
	if len(cli.Port) < 1 && !cli.ListInterfaces && !cli.Version {
		log.Fatalf("Please specify one or more --port")
	}

	if cli.Logfile != "stderr" {
		file, err := os.OpenFile(cli.Logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			log.WithError(err).Fatalf("Unable to open log file: %s", cli.Logfile)
		}
		log.SetOutput(file)
	}

	return cli
}
