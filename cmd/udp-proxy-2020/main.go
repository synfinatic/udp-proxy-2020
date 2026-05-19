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

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
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

	if cli.GraphPipeline != "" {
		if err := GenerateDotFile(pipelines, cli.GraphPipeline); err != nil {
			slog.Error("Failed to generate dot file", "error", err)
			os.Exit(1)
		}
		slog.Info("Successfully generated Graphviz dot file", "path", cli.GraphPipeline)
		os.Exit(0)
	}

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

	// Start UDP listeners if --no-listen is not set
	if !cli.NoListen {
		startUDPListeners(ctx, &wg, cli)
	}

	slog.Info("All pipelines started")
	wg.Wait()
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

	var (
		pipelines []*proxy.Pipeline
		states    []ifaceState
	)

	for _, iname := range interfaces {
		state, pipeline, err := setupInterfacePipeline(cli, dm, registry, iname, timeout)
		if err != nil {
			return nil, nil, err
		}
		states = append(states, state)
		pipelines = append(pipelines, pipeline)
	}

	if err := attachCrossInterfaceSinks(states, func(src, dst ifaceState) error {
		return setupCrossInterfaceSink(cli, dm, registry, src, dst)
	}); err != nil {
		return nil, nil, err
	}

	return pipelines, registries, nil
}

// setupInterfacePipeline initializes a pipeline for a single interface and returns its state and pipeline.
func setupInterfacePipeline(cli CLI, dm *proxy.DeviceManager, registry *stages.RegistryProcessor, iname string, timeout time.Duration) (ifaceState, *proxy.Pipeline, error) {
	netif, err := net.InterfaceByName(iname)
	if err != nil {
		slog.Error("Interface not found", "interface", iname, "error", err)
		return ifaceState{}, nil, fmt.Errorf("interface not found: %s", iname)
	}

	source, err := stages.NewPcapSource(dm, iname, (netif.Flags&net.FlagBroadcast) == 0, timeout)
	if err != nil {
		slog.Error("Failed to open interface", "interface", iname, "error", err)
		return ifaceState{}, nil, fmt.Errorf("failed to open interface: %s", iname)
	}
	rhandle := source.Handle()

	addrs, err := dm.GetAddresses(iname)
	if err != nil {
		slog.Error("Failed to get addresses for interface", "interface", iname, "error", err)
		return ifaceState{}, nil, fmt.Errorf("failed to get addresses for interface: %s", iname)
	}
	filter := config.BuildBPFFilter(cli.Port, addrs)
	if err := rhandle.SetBPFFilter(filter); err != nil {
		slog.Error("Failed to set BPF filter", "interface", iname, "error", err)
		return ifaceState{}, nil, fmt.Errorf("failed to set BPF filter for interface: %s", iname)
	}

	pipeline := proxy.NewPipeline(source)
	pipeline.AddProcessor(&stages.FilterProcessor{Iname: iname})
	if cli.Decode {
		pipeline.AddProcessor(stages.NewDecodeProcessor(iname, stages.DirectionInbound, os.Stdout))
	}
	pipeline.AddProcessor(&stages.RegistryLearnerProcessor{Registry: registry, Iname: iname})

	if cli.Pcap {
		if err := addPcapFileSink(pipeline, cli.PcapPath, fmt.Sprintf("udp-proxy-in-%s.pcap", iname), rhandle.LinkType()); err != nil {
			return ifaceState{}, nil, err
		}
	}

	bcast, err := discoverBroadcastAddress(dm, netif, addrs, cli, iname)
	if err != nil {
		return ifaceState{}, nil, err
	}

	state := ifaceState{
		name:      iname,
		netif:     netif,
		source:    source,
		pipeline:  pipeline,
		broadcast: (netif.Flags & net.FlagBroadcast) != 0,
		bcastIP:   bcast,
	}
	return state, pipeline, nil
}

// addPcapFileSink adds a PcapFileSink to the pipeline.
func addPcapFileSink(pipeline *proxy.Pipeline, pcapPath, filename string, linkType layers.LinkType) error {
	fPath := filepath.Join(pcapPath, filename)
	f, err := os.Create(fPath)
	if err != nil {
		slog.Error("Failed to create pcap file", "error", err)
		return fmt.Errorf("failed to create pcap file: %s", fPath)
	}
	w := pcapgo.NewWriter(f)
	if err = w.WriteFileHeader(65536, linkType); err != nil {
		f.Close()
		slog.Error("Failed to write pcap file header", "error", err)
		return fmt.Errorf("failed to write pcap file header: %s", fPath)
	}
	pipeline.AddSink(&stages.PcapFileSink{Writer: w, File: f})
	return nil
}

// discoverBroadcastAddress finds the broadcast address for an interface.
func discoverBroadcastAddress(dm *proxy.DeviceManager, netif *net.Interface, addrs []pcap.InterfaceAddress, cli CLI, iname string) (net.IP, error) {
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
		return nil, fmt.Errorf("failed to discover broadcast address for interface: %s", iname)
	}
	return bcast, nil
}

// setupCrossInterfaceSink attaches a cross-interface sink between two ifaceStates.
func setupCrossInterfaceSink(cli CLI, dm *proxy.DeviceManager, registry *stages.RegistryProcessor, src, dst ifaceState) error {
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
		if err := addRoutePcapFileSink(route, cli.PcapPath, src.name, dst.name, transmitter.Writer.LinkType()); err != nil {
			return err
		}
	}

	route.Sinks = append(route.Sinks, transmitter)
	src.pipeline.AddSink(route)
	return nil
}

// addRoutePcapFileSink adds a PcapFileSink to a RouteSink.
func addRoutePcapFileSink(route *stages.RouteSink, pcapPath, srcName, dstName string, linkType layers.LinkType) error {
	fPath := filepath.Join(pcapPath, fmt.Sprintf("udp-proxy-out-%s-to-%s.pcap", srcName, dstName))
	f, err := os.Create(fPath)
	if err != nil {
		slog.Error("Failed to create outbound pcap file", "error", err)
		return fmt.Errorf("failed to create outbound pcap file: %s", fPath)
	}
	w := pcapgo.NewWriter(f)
	if err = w.WriteFileHeader(65536, linkType); err != nil {
		f.Close()
		slog.Error("Failed to write outbound pcap file header", "error", err)
		return fmt.Errorf("failed to write outbound pcap file header: %s", fPath)
	}
	route.Sinks = append(route.Sinks, &stages.PcapFileSink{Writer: w, File: f})
	return nil
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
