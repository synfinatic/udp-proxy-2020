package stages

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
	proxyrewrite "github.com/synfinatic/udp-proxy-2020/internal/proxy/rewrite"
)

// RouteSink makes routing decisions (determines target destinations) and fans out packets
// to those targets. Per-target packet rewriting and nested sink processing is handled
// by writeToTarget().
type RouteSink struct {
	Iname            string
	Broadcast        bool
	BroadcastAddress net.IP
	HardwareAddr     net.HardwareAddr
	Registry         *RegistryProcessor
	LinkType         proxy.PacketWriter
	Processors       []proxy.Processor
	Sinks            []proxy.Sink
}

type routeTarget struct {
	IP                net.IP
	MAC               net.HardwareAddr
	BroadcastDestMAC  bool
	AllowBroadcastMAC bool
}

func (s *RouteSink) Name() string {
	return fmt.Sprintf("RouteSink(%s)", s.Iname)
}

func (s *RouteSink) Write(pkt *proxy.Packet) error {
	if pkt == nil {
		return nil
	}

	targets := s.targetsForPacket(pkt)
	if len(targets) == 0 {
		slog.Debug("0 targets on interface to forward packet, dropping", "interface", s.Iname)
		return nil
	}

	for _, target := range targets {
		s.writeToTarget(pkt, target)
	}

	return nil
}

// writeToTarget handles per-target packet rewriting and forwarding to nested sinks.
func (s *RouteSink) writeToTarget(pkt *proxy.Packet, target routeTarget) {
	rewritten, err := proxyrewrite.PacketForEgress(pkt, proxyrewrite.Options{
		TargetIP:               target.IP,
		TargetMAC:              target.MAC,
		SourceMAC:              s.HardwareAddr,
		EgressLinkType:         s.LinkType.LinkType(),
		AllowBroadcastDstMAC:   target.AllowBroadcastMAC,
		ForceBroadcastDestMAC:  target.BroadcastDestMAC,
		OutputArrivalInterface: s.Iname,
	})
	if err != nil {
		slog.Warn("Unable to rewrite packet", "to_interface", s.Iname, "dst_ip", target.IP, "error", err)
		return
	}

	continueProcessing := true
	for _, proc := range s.Processors {
		keep, err := proc.Process(rewritten)
		if err != nil {
			slog.Error("Route sink processor error", "processor", proc.Name(), "to_interface", s.Iname, "error", err)
			continueProcessing = false
			break
		}
		if !keep {
			continueProcessing = false
			break
		}
	}
	if !continueProcessing {
		return
	}

	for _, sink := range s.Sinks {
		if err := sink.Write(rewritten); err != nil {
			slog.Error("Route sink write error", "sink", sink.Name(), "to_interface", s.Iname, "error", err)
		}
	}
}

func (s *RouteSink) targetsForPacket(pkt *proxy.Packet) []routeTarget {
	targets := make([]routeTarget, 0)

	if s.Registry != nil {
		// Route toward clients known on the egress interface.
		clients := s.Registry.GetClientsForInterface(s.Iname)
		for _, client := range clients {
			targets = append(targets, routeTarget{
				IP:                client.IP,
				MAC:               client.MAC,
				AllowBroadcastMAC: s.Broadcast,
			})
		}
		if len(targets) > 0 {
			return targets
		}
	}

	if s.Broadcast {
		targets = append(targets, routeTarget{
			IP:               s.BroadcastAddress,
			BroadcastDestMAC: true,
		})
	}

	return targets
}

func (s *RouteSink) Close() error {
	var firstErr error
	for _, sink := range s.Sinks {
		if err := sink.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
