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

var newRegistryProcessorByInterface = stages.NewRegistryProcessorByInterface

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
	NoListen       bool     `kong:"help='Do not listen locally on UDP ports'"`
	Decode         bool     `kong:"help='Print packet decodes to stdout similar to tcpdump -e'"`
	Pcap           bool     `kong:"short='P',help='Generate pcap files for debugging'"`
	PcapPath       string   `kong:"short='d',default='/root',help='Directory to write debug pcap files'"`
	ListInterfaces bool     `kong:"help='List available interfaces and exit'"`
	Version        bool     `kong:"short='v',help='Print version information'"`
}

func main() {
	cli := parseArgs()
	setupLogging(cli)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	timeout, ttl := getDurations(cli)

	dm, err := proxy.NewDeviceManager()
	if err != nil {
		slog.Error("Failed to initialize device manager", "error", err)
		os.Exit(1)
	}
	defer dm.CloseHandles()

	if cli.ListInterfaces {
		dm.ListInterfaces()
		os.Exit(0)
	}

	pipelines, registries, err := setupPipelines(cli, dm, timeout, ttl)
	if err != nil {
		slog.Error("Failed to set up pipelines", "error", err)
		return
	}

	// Start registry cleanup ticker
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, r := range registries {
					r.Cleanup()
				}
			}
		}
	}()

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

	slog.Info("All pipelines started")
	wg.Wait()
}

func setupLogging(cli CLI) {
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
}

func getDurations(cli CLI) (time.Duration, time.Duration) {
	timeout := config.ParseTimeout(cli.Timeout)
	if cli.CacheTTL <= 0 {
		slog.Error("CacheTTL must be a positive number of minutes")
		os.Exit(1)
	}
	ttl, err := time.ParseDuration(fmt.Sprintf("%dm", cli.CacheTTL))
	if err != nil {
		slog.Error("Invalid CacheTTL", "value", cli.CacheTTL, "error", err)
		os.Exit(1)
	}
	return timeout, ttl
}

type ifaceState struct {
	name      string
	netif     *net.Interface
	source    *stages.PcapSource
	pipeline  *proxy.Pipeline
	broadcast bool
	bcastIP   net.IP
}

func attachCrossInterfaceSinks(states []ifaceState, addSink func(src, dst ifaceState) error) error {
	for _, src := range states {
		for _, dst := range states {
			if src.name == dst.name {
				continue
			}
			if err := addSink(src, dst); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildSharedRegistries(ttl time.Duration, fixedIPs map[string][]string) ([]*stages.RegistryProcessor, error) {
	registry, err := newRegistryProcessorByInterface(ttl, fixedIPs)
	if err != nil {
		return nil, err
	}

	return []*stages.RegistryProcessor{registry}, nil
}

func setupPipelines(cli CLI, dm *proxy.DeviceManager, timeout, ttl time.Duration) ([]*proxy.Pipeline, []*stages.RegistryProcessor, error) {
	var pipelines []*proxy.Pipeline

	checkDuplicateInterfaces(cli.Interface)

	interfaces := cli.Interface
	if cli.DeliverLocal {
		lb := dm.GetLoopback()
		if lb == "" {
			slog.Error("Unable to find loopback interface")
			return nil, nil, fmt.Errorf("loopback interface not found")
		}
		interfaces = append(interfaces, lb)
	}

	fixedIPs := getFixedIPs(cli, dm)
	registries, err := buildSharedRegistries(ttl, fixedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid fixed IP configuration: %w", err)
	}
	registry := registries[0]

	states := make([]ifaceState, 0, len(interfaces))

	for _, iname := range interfaces {
		netif, err := net.InterfaceByName(iname)
		if err != nil {
			slog.Error("Interface not found", "interface", iname, "error", err)
			return nil, nil, fmt.Errorf("interface not found: %s", iname)
		}

		source, err := stages.NewPcapSource(dm, iname, (netif.Flags&net.FlagBroadcast) == 0, timeout)
		if err != nil {
			slog.Error("Failed to open interface", "interface", iname, "error", err)
			return nil, nil, fmt.Errorf("failed to open interface: %s", iname)
		}
		rhandle := source.Handle()

		// Set BPF filter
		addrs, err := dm.GetAddresses(iname)
		if err != nil {
			slog.Error("Failed to get addresses for interface", "interface", iname, "error", err)
			return nil, nil, fmt.Errorf("failed to get addresses for interface: %s", iname)
		}
		filter := config.BuildBPFFilter(cli.Port, addrs)
		if err := rhandle.SetBPFFilter(filter); err != nil {
			slog.Error("Failed to set BPF filter", "interface", iname, "error", err)
			return nil, nil, fmt.Errorf("failed to set BPF filter for interface: %s", iname)
		}

		pipeline := proxy.NewPipeline(source)
		pipeline.AddProcessor(&stages.FilterProcessor{Iname: iname})
		if cli.Decode {
			pipeline.AddProcessor(stages.NewDecodeProcessor(iname, stages.DirectionInbound, os.Stdout))
		}
		pipeline.AddProcessor(&stages.RegistryLearnerProcessor{Registry: registry, Iname: iname})

		// Pcap file logging
		if cli.Pcap {
			fPath := filepath.Join(cli.PcapPath, fmt.Sprintf("udp-proxy-in-%s.pcap", iname))
			f, err := os.Create(fPath)
			if err != nil {
				slog.Error("Failed to create pcap file", "error", err)
				return nil, nil, fmt.Errorf("failed to create pcap file: %s", fPath)
			}
			w := pcapgo.NewWriter(f)
			if err = w.WriteFileHeader(65536, rhandle.LinkType()); err != nil {
				f.Close()
				slog.Error("Failed to write pcap file header", "error", err)
				return nil, nil, fmt.Errorf("failed to write pcap file header: %s", fPath)
			}
			pipeline.AddSink(&stages.PcapFileSink{Writer: w, File: f})
		}

		// Broadcast address discovery (reuse addrs already fetched above)
		var bcast net.IP
		for _, addr := range addrs {
			if addr.IP.To4() != nil && addr.Broadaddr != nil {
				bcast = addr.Broadaddr
				break
			}
		}
		if bcast == nil && (netif.Flags&net.FlagBroadcast) != 0 {
			slog.Warn("No broadcast address found for interface", "interface", iname)
		}

		if cli.DeliverLocal && iname == dm.GetLoopback() {
			bcast = net.ParseIP("127.0.0.1")
		}

		if bcast == nil && (netif.Flags&net.FlagBroadcast) != 0 {
			slog.Error("Failed to discover broadcast address for interface", "interface", iname)
			return nil, nil, fmt.Errorf("failed to discover broadcast address for interface: %s", iname)
		}

		states = append(states, ifaceState{
			name:      iname,
			netif:     netif,
			source:    source,
			pipeline:  pipeline,
			broadcast: (netif.Flags & net.FlagBroadcast) != 0,
			bcastIP:   bcast,
		})

		pipelines = append(pipelines, pipeline)
	}

	if err := attachCrossInterfaceSinks(states, func(src, dst ifaceState) error {
		transmitter, err := stages.NewTransmitterSink(dm, dst.name)
		if err != nil {
			slog.Error("Failed to create transmitter sink", "source_interface", src.name, "target_interface", dst.name, "error", err)
			return fmt.Errorf("failed to create transmitter sink from %s to %s", src.name, dst.name)
		}

		route := &stages.RouteSink{
			Iname:            dst.name,
			Broadcast:        dst.broadcast,
			BroadcastAddress: dst.bcastIP,
			HardwareAddr:     dst.netif.HardwareAddr,
			Registry:         registry,
			LinkType:         transmitter.Writer,
		}

		if cli.Decode {
			route.Processors = append(route.Processors, stages.NewDecodeProcessor(dst.name, stages.DirectionOutbound, os.Stdout))
		}

		if cli.Pcap {
			fPath := filepath.Join(cli.PcapPath, fmt.Sprintf("udp-proxy-out-%s-to-%s.pcap", src.name, dst.name))
			f, err := os.Create(fPath)
			if err != nil {
				slog.Error("Failed to create outbound pcap file", "error", err)
				return fmt.Errorf("failed to create outbound pcap file: %s", fPath)
			}
			w := pcapgo.NewWriter(f)
			if err = w.WriteFileHeader(65536, transmitter.Writer.LinkType()); err != nil {
				f.Close()
				slog.Error("Failed to write outbound pcap file header", "error", err)
				return fmt.Errorf("failed to write outbound pcap file header: %s", fPath)
			}
			route.Sinks = append(route.Sinks, &stages.PcapFileSink{Writer: w, File: f})
		}

		route.Sinks = append(route.Sinks, transmitter)
		src.pipeline.AddSink(route)
		return nil
	}); err != nil {
		return nil, nil, err
	}

	return pipelines, registries, nil
}

func checkDuplicateInterfaces(ifaces []string) {
	seenInterfaces := make(map[string]bool)
	for _, iname := range ifaces {
		if seenInterfaces[iname] {
			slog.Error("Duplicate interface specified", "interface", iname)
			os.Exit(1)
		}
		seenInterfaces[iname] = true
	}
}

func getFixedIPs(cli CLI, dm *proxy.DeviceManager) map[string][]string {
	fixedIPs := make(map[string][]string)
	for _, f := range cli.FixedIp {
		parts := strings.Split(f, "@")
		if len(parts) != 2 {
			slog.Error("Invalid fixed IP format. Expected interface@ip", "value", f)
			os.Exit(1)
		}
		if net.ParseIP(parts[1]) == nil {
			slog.Error("Invalid fixed IP address", "ip", parts[1])
			os.Exit(1)
		}
		// Check if the interface is in the whitelist or is loopback (if deliver-local is on)
		validIface := false
		for _, i := range cli.Interface {
			if i == parts[0] {
				validIface = true
				break
			}
		}
		if !validIface && (!cli.DeliverLocal || parts[0] != dm.GetLoopback()) {
			slog.Error("Fixed IP interface must be active", "interface", parts[0])
			os.Exit(1)
		}
		fixedIPs[parts[0]] = append(fixedIPs[parts[0]], parts[1])
	}
	return fixedIPs
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
