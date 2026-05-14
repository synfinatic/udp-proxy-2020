package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gopacket/gopacket/pcapgo"
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

func main() {
	cli := parseArgs()
	// Setup Logging
	var level slog.Level
	switch cli.Level {
	case "trace":
		level = slog.LevelDebug - 4
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "info":
		level = slog.LevelInfo
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cli.LogLines,
	}

	var handler slog.Handler
	if cli.Logfile != "stderr" {
		file, err := os.OpenFile(cli.Logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to open log file %s: %v\n", cli.Logfile, err)
			os.Exit(1)
		}
		handler = slog.NewTextHandler(file, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	timeout := config.ParseTimeout(cli.Timeout)
	ttl, _ := time.ParseDuration(fmt.Sprintf("%dm", cli.CacheTTL))

	dm, err := proxy.NewDeviceManager()
	if err != nil {
		slog.Error("Failed to initialize device manager", "error", err)
		os.Exit(1)
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
			slog.Error("Unable to find loopback interface")
			os.Exit(1)
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
			slog.Error("Interface not found", "interface", iname, "error", err)
			os.Exit(1)
		}

		handle, err := dm.CreateHandle(iname, (netif.Flags&net.FlagBroadcast) == 0, timeout)
		if err != nil {
			slog.Error("Failed to open interface", "interface", iname, "error", err)
			os.Exit(1)
		}

		// Set BPF filter
		addrs, _ := dm.GetAddresses(iname)
		filter := config.BuildBPFFilter(cli.Port, addrs)
		if err := handle.SetBPFFilter(filter); err != nil {
			slog.Error("Failed to set BPF filter", "interface", iname, "error", err)
			os.Exit(1)
		}

		registry := stages.NewRegistryProcessor(ttl, fixedIPs[iname])

		pipeline := proxy.NewPipeline(stages.NewPcapSource(handle, iname))
		pipeline.AddProcessor(&stages.FilterProcessor{Iname: iname})
		pipeline.AddProcessor(registry)

		// Pcap file logging
		if cli.Pcap {
			fPath := filepath.Join(cli.PcapPath, fmt.Sprintf("udp-proxy-in-%s.pcap", iname))
			f, err := os.Create(fPath)
			if err != nil {
				slog.Error("Failed to create pcap file", "error", err)
				os.Exit(1)
			}
			w := pcapgo.NewWriter(f)
			err = w.WriteFileHeader(65536, handle.LinkType())
			if err != nil {
				slog.Error("Failed to write pcap file header", "error", err)
				os.Exit(1)
			}
			pipeline.AddSink(&stages.PcapFileSink{Writer: w})
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
				slog.Error("Pipeline error", "error", err)
			}
		}(p)
	}

	for _, t := range transmitters {
		go t.Run()
	}

	slog.Info("All pipelines started")
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

	if len(cli.Interface) < 2 && !cli.ListInterfaces && !cli.Version {
		fmt.Fprintf(os.Stderr, "Please specify two or more --interface\n")
		os.Exit(1)
	}
	if len(cli.Port) < 1 && !cli.ListInterfaces && !cli.Version {
		fmt.Fprintf(os.Stderr, "Please specify one or more --port\n")
		os.Exit(1)
	}

	return cli
}
