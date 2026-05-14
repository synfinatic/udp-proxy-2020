# UDP Proxy 2020 API Documentation

The `udp-proxy-2020` refactor introduces a modular, pipeline-based architecture for processing and distributing UDP packets across multiple network interfaces. This document explains the high-level architecture and how to use the new internal library.

## Architecture Overview

The system is built on four primary components:

1. **DeviceManager**: Handles discovery and initialization of physical network interfaces using `libpcap`.
2. **PacketBus**: A subscription-based message bus that routes packets from one interface's pipeline to all other active interface pipelines.
3. **Pipeline**: Orchestrates the flow of a single packet through a series of stages.
4. **Stages**: Modular components that perform specific tasks:
    * **Sources**: Capture packets (e.g., `PcapSource`).
    * **Processors**: Filter, transform, or learn from packets (e.g., `FilterProcessor`, `RegistryProcessor`).
    * **Sinks**: Terminal points for packets (e.g., `ForwardingSink`, `TransmitterSink`, `PcapFileSink`).

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
    "time"
    "github.com/synfinatic/udp-proxy-2020/internal/proxy"
    "github.com/synfinatic/udp-proxy-2020/internal/proxy/stages"
    "github.com/synfinatic/udp-proxy-2020/internal/config"
)

func main() {
    ctx := context.Background()
    dm, _ := proxy.NewDeviceManager()
    bus := proxy.NewPacketBus()
    
    ports := []int32{9003}
    interfaces := []string{"eth0", "eth1", "vpn0"}
    
    // Map to hold fixed IPs per interface
    fixedIPs := map[string][]string{
        "vpn0": {"192.168.10.50"}, // Hard-coded host on vpn0
    }

    for _, iname := range interfaces {
        // 1. Create Handle
        handle, _ := dm.CreateHandle(iname, true, 250 * time.Millisecond)
        
        // 2. Set BPF Filter for port 9003
        addrs, _ := dm.GetAddresses(iname)
        filter := config.BuildBPFFilter(ports, addrs)
        handle.SetBPFFilter(filter)

        // 3. Initialize Registry (handles learning and fixed IPs)
        registry := stages.NewRegistryProcessor(180 * time.Minute, fixedIPs[iname])

        // 4. Create Pipeline Source
        pipeline := proxy.NewPipeline(stages.NewPcapSource(handle, iname))

        // 5. Add Processors
        pipeline.AddProcessor(&stages.FilterProcessor{Iname: iname})
        pipeline.AddProcessor(registry)

        // 6. Add Forwarding Sink (Sends to other interfaces via Bus)
        pipeline.AddSink(&stages.ForwardingSink{
            Feed:     bus,
            Iname:    iname,
            LinkType: handle.LinkType(),
        })

        // 7. Subscribe to the Bus for outgoing packets
        busChan := make(chan proxy.BusMessage, 100)
        bus.Subscribe(iname, busChan)

        // 8. Start Transmitter (Physical output)
        transmitter := &stages.TransmitterSink{
            Handle:       handle,
            Iname:        iname,
            PacketBus:    busChan,
            Registry:     registry, // Uses registry to find where to send
            // ... hardware addr and broadcast IP discovery omitted for brevity
        }
        go transmitter.Run()

        // 9. Run the pipeline logic
        go pipeline.Run(ctx)
    }

    select {} // Keep alive
}
```

## How It Works

1. A packet arrives on `eth0:9003`.
2. The `eth0` Pipeline captures it via `PcapSource`.
3. The `RegistryProcessor` on `eth0` learns the sender's IP.
4. The `ForwardingSink` publishes the packet to the `PacketBus`.
5. The `PacketBus` sends the packet to the subscription channels for `eth1` and `vpn0`.
6. The `TransmitterSink` for `vpn0` receives the message. It checks its `RegistryProcessor`.
7. The `vpn0` Registry finds the fixed IP `192.168.10.50`.
8. The `TransmitterSink` builds a new UDP packet and sends it out of `vpn0` to `192.168.10.50`.
