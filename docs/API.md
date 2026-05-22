# UDP Proxy 2020 API Documentation

The `udp-proxy-2020` refactor introduces a modular, pipeline-based architecture for processing and distributing UDP packets across multiple network interfaces. This document explains the high-level architecture and how to use the new internal library.

## Architecture Overview

The system is built on four primary components:

1. **DeviceManager**: Handles discovery and initialization of physical network interfaces using `libpcap`.
2. **PacketBus**: A subscription-based message bus that routes packets from one interface's pipeline to all other active interface pipelines.
3. **Pipeline**: Orchestrates the flow of a single packet through a series of stages. Pipeline setup is modular and intentionally declarative, with sources, processors, and sinks added in explicit order.
4. **Stages**: Modular components that perform specific tasks:
    * **Sources**: Capture packets (e.g., `PcapSource`).
    * **Processors**: Filter, transform, or learn from packets (e.g., `FilterProcessor`, `RegistryProcessor`).
    * **Sinks**: Terminal points for packets (e.g., `ForwardingSink`, `RouteSink`, `TransmitterSink`, `PcapFileSink`).

## Key Features

* **Modular Pipeline**: Easily add new packet processing logic (e.g., packet modification, logging, or custom filtering).
* **Concurrency Safe**: Each interface runs its own pipeline in a dedicated goroutine.
* **Support for Multi-DLT**: Handles different Link Types (Ethernet, BSD Loopback, Raw IP) seamlessly across source and destination interfaces.
* **Registry Management**: Automatically learns client IPs or supports fixed (immortal) host entries for specific interfaces.

---

## Example: 3-Interface Configuration

In this example, we configure the proxy for three interfaces: `eth0`, `eth1`, and `vpn0`.

* **Global Port**: `udp/9003`
* **Special Requirement**: `vpn0` has a hard-coded host at `192.168.10.50` that should always receive packets even if it hasn't sent any.

### Code Example

```go
package main

import (
    "context"
    "net"
    "time"

    "github.com/synfinatic/udp-proxy-2020/internal/config"
    "github.com/synfinatic/udp-proxy-2020/internal/proxy"
    "github.com/synfinatic/udp-proxy-2020/internal/proxy/stages"
)

func main() {
    ctx := context.Background()
    dm, _ := proxy.NewDeviceManager()

    ports := []int32{9003}
    interfaces := []string{"eth0", "eth1", "vpn0"}
    fixedIPs := map[string][]string{"vpn0": {"192.168.10.50"}}

    registry, _ := stages.NewRegistryProcessorByInterface(180*time.Minute, fixedIPs)
    pipelines := make(map[string]*proxy.Pipeline)
    states := make(map[string]struct {
        netif  *net.Interface
        bcast  net.IP
        source *stages.PcapSource
    })

    for _, iname := range interfaces {
        netif, _ := net.InterfaceByName(iname)

        // 1. Open source capture and set BPF
        source, _ := stages.NewPcapSource(dm, iname, (netif.Flags&net.FlagBroadcast) == 0, 250*time.Millisecond)
        handle := source.Handle()
        addrs, _ := dm.GetAddresses(iname)
        filter := config.BuildBPFFilter(ports, addrs)
        handle.SetBPFFilter(filter)

        // 2. Build inbound pipeline
        pipeline := proxy.NewPipeline(source)
        pipeline.AddProcessor(&stages.FilterProcessor{Iname: iname})
        pipeline.AddProcessor(&stages.RegistryLearnerProcessor{Registry: registry, Iname: iname})

        pipelines[iname] = pipeline
        states[iname] = struct {
            netif  *net.Interface
            bcast  net.IP
            source *stages.PcapSource
        }{netif: netif, bcast: addrs[0].Broadaddr, source: source}
    }

    // 3. Wire cross-interface RouteSink -> TransmitterSink fanout
    for srcName, srcPipeline := range pipelines {
        for dstName, dst := range states {
            if srcName == dstName {
                continue
            }

            tx, _ := stages.NewTransmitterSink(dm, dstName)
            route := &stages.RouteSink{
                Iname:            dstName,
                Broadcast:        (dst.netif.Flags & net.FlagBroadcast) != 0,
                BroadcastAddress: dst.bcast,
                HardwareAddr:     dst.netif.HardwareAddr,
                Registry:         registry,
                LinkType:         tx.Writer.LinkType(),
                Sinks:            []proxy.Sink{tx},
            }
            srcPipeline.AddSink(route)
        }
    }

    // 4. Run inbound pipelines
    for _, pipeline := range pipelines {
        go pipeline.Run(ctx)
    }

    select {} // Keep alive
}
```

## How It Works

1. A packet arrives on `eth0:9003`.
2. The `eth0` Pipeline captures it via `PcapSource`.
3. The `RegistryProcessor` on `eth0` learns the sender's IP.
4. The source pipeline's `RouteSink(vpn0)` chooses one or more destination targets using the shared `RegistryProcessor`.
5. For each target, `RouteSink` rewrites the packet for `vpn0` egress and forwards it to downstream sinks.
6. The `TransmitterSink(vpn0)` writes the rewritten packet to the `vpn0` interface.
7. If no learned clients exist on `vpn0`, broadcast fallback can still deliver the packet (when enabled).
8. Fixed entries such as `vpn0@192.168.10.50` are always considered valid route targets.

---

## Pipeline Stages: Sources, Processors, and Sinks

### Sources

* **PcapSource**: Reads packets from a libpcap handle (network interface). Feeds packets into the pipeline for processing.

### Processors

* **FilterProcessor**: Drops packets that are not valid UDP/IPv4. Ensures only relevant packets are processed downstream.
* **RegistryProcessor**: Learns and caches client IPs per interface. Supports both dynamic learning and fixed (immortal) IPs for always-on delivery.
* **DecodeProcessor**: Prints a one-line summary of each packet (like `tcpdump -e`) for debugging or logging. Can be enabled per interface for inbound and/or outbound directions.
* **TransformProcessor**: Modifies packet headers (e.g., destination IP) and recalculates checksums. Used for NAT or packet rewriting scenarios.
* **RewriteProcessor**: Applies L2/L3 changes to outbound packets, such as updating MAC addresses or forcing broadcast destination MACs.

### Sinks

* **ForwardingSink**: Publishes packets to the `PacketBus`, distributing them to other interface pipelines.
* **RouteSink**: Fans out packets per destination, rewrites them as needed, and forwards to downstream sinks (e.g., for multi-destination delivery).
* **TransmitterSink**: Sends packets to a physical network interface using a libpcap writer handle.
* **PcapFileSink**: Writes packets to a `.pcap` file for offline analysis or debugging. Can be enabled for inbound and outbound traffic capture.

---

## Adding Custom Stages

To add a new processor or sink, implement the appropriate interface (`Processor` or `Sink`) and insert it into the pipeline using `AddProcessor` or `AddSink` during pipeline assembly. The current setup path is organized around small helper functions for per-interface and cross-interface wiring, which makes it easier to add or swap stages without growing setup complexity. See the `stages/` directory for examples.
