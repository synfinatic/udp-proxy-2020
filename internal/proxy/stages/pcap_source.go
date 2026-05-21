package stages

import (
	"context"
	"log/slog"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

const (
	reconnectInitialDelay = 1 * time.Second
	reconnectMaxDelay     = 30 * time.Second
)

// PcapSource reads packets from a libpcap handle.
type PcapSource struct {
	dm           *proxy.DeviceManager
	handle       *pcap.Handle
	packetSource *gopacket.PacketSource
	packets      chan gopacket.Packet
	iname        string
	promisc      bool
	timeout      time.Duration

	createReaderHandle func(iname string, promisc bool, timeout time.Duration) (*pcap.Handle, error)
	closeReaderHandle  func(iname string, direction proxy.PcapHandleDirection) error
	interfaceAvailable func(iname string) bool
	newPacketSource    func(handle *pcap.Handle) (*gopacket.PacketSource, chan gopacket.Packet)
	reconnectSignal    chan struct{}
	monitorCancel      context.CancelFunc
}

// NewPcapSource creates a new PcapSource.
func NewPcapSource(dm *proxy.DeviceManager, iname string, promisc bool, timeout time.Duration) (*PcapSource, error) {
	handle, err := dm.CreateReaderHandle(iname, promisc, timeout)
	if err != nil {
		return nil, err
	}
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	source := &PcapSource{
		dm:                 dm,
		handle:             handle,
		packetSource:       packetSource,
		packets:            packetSource.Packets(),
		iname:              iname,
		promisc:            promisc,
		timeout:            timeout,
		createReaderHandle: dm.CreateReaderHandle,
		closeReaderHandle:  dm.Close,
		interfaceAvailable: dm.InterfaceAvailable,
		newPacketSource: func(h *pcap.Handle) (*gopacket.PacketSource, chan gopacket.Packet) {
			ps := gopacket.NewPacketSource(h, h.LinkType())
			return ps, ps.Packets()
		},
		reconnectSignal: make(chan struct{}, 1),
		monitorCancel:   nil,
	}
	source.startInterfaceMonitor()
	return source, nil
}

func (s *PcapSource) startInterfaceMonitor() {
	if s.interfaceAvailable == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.monitorCancel = cancel

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		interfaceWasDown := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				available := s.interfaceAvailable(s.iname)
				if !available {
					if !interfaceWasDown {
						slog.Warn("interface appears down, awaiting recovery",
							slog.String("interface", s.iname))
					}
					interfaceWasDown = true
					continue
				}

				if interfaceWasDown {
					slog.Info("interface is back, reconnecting capture handle",
						slog.String("interface", s.iname))
					select {
					case s.reconnectSignal <- struct{}{}:
					default:
					}
					interfaceWasDown = false
				}
			}
		}
	}()
}

func (s *PcapSource) Handle() *pcap.Handle {
	return s.handle
}

// reconnect closes the stale pcap handle and waits for the interface to come
// back up, then opens a fresh handle and resets the packet channel. It returns
// when reconnection succeeds or ctx is cancelled.
func (s *PcapSource) reconnect(ctx context.Context, reason string) error {
	slog.Warn("pcap source lost, waiting for interface to come back",
		slog.String("interface", s.iname),
		slog.String("reason", reason))

	// Close the old handle; ignore "not found" — it may already be gone.
	_ = s.closeReaderHandle(s.iname, proxy.Reader)

	delay := reconnectInitialDelay
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		handle, err := s.createReaderHandle(s.iname, s.promisc, s.timeout)
		if err != nil {
			slog.Debug("interface not yet available, retrying",
				slog.String("interface", s.iname),
				slog.String("reason", reason),
				slog.Duration("retry_in", delay),
				slog.String("error", err.Error()))
			if delay < reconnectMaxDelay {
				delay *= 2
				if delay > reconnectMaxDelay {
					delay = reconnectMaxDelay
				}
			}
			continue
		}

		packetSource, packets := s.newPacketSource(handle)
		s.handle = handle
		s.packetSource = packetSource
		s.packets = packets
		slog.Info("interface reconnected",
			slog.String("interface", s.iname),
			slog.String("reason", reason))
		return nil
	}
}

// Read reads the next packet from the PCAP handle. If the packet channel is
// closed because the interface/handle disappeared, it reconnects until the
// handle is restored or ctx is cancelled.
func (s *PcapSource) Read(ctx context.Context) (*proxy.Packet, error) {
	reconnectSignal := s.reconnectSignal

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-reconnectSignal:
			if err := s.reconnect(ctx, "interface_down_up"); err != nil {
				return nil, err
			}
			continue
		case p, ok := <-s.packets:
			if !ok {
				// Interface went away — attempt to reconnect.
				if err := s.reconnect(ctx, "packets_channel_closed"); err != nil {
					return nil, err
				}
				// Restart the select with the new packets channel.
				continue
			}

			return &proxy.Packet{
				Raw:              p.Data(),
				Metadata:         p.Metadata().CaptureInfo,
				Packet:           p,
				ArrivalInterface: s.iname,
			}, nil
		}
	}
}

// Close closes the underlying PCAP handle
func (s *PcapSource) Close() error {
	if s.monitorCancel != nil {
		s.monitorCancel()
	}
	return s.dm.Close(s.iname, proxy.Reader)
}

func (s *PcapSource) Name() string {
	return "PcapSource:" + s.iname
}
