package stages

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

// TransmitterSink sends packets to a physical interface.
type TransmitterSink struct {
	dm           *proxy.DeviceManager
	mu           sync.Mutex // protects Writer and reconnecting
	Writer       proxy.PacketWriter
	Iname        string
	reconnecting bool
	ctx          context.Context
	cancel       context.CancelFunc
	createWriter func(iname string) (proxy.PacketWriter, error)
}

// NewTransmitterSink creates a new TransmitterSink.
func NewTransmitterSink(dm *proxy.DeviceManager, iname string) (*TransmitterSink, error) {
	handle, err := dm.CreateIsolatedWriterHandle(iname)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &TransmitterSink{
		dm:     dm,
		Writer: handle,
		Iname:  iname,
		ctx:    ctx,
		cancel: cancel,
		createWriter: func(iname string) (proxy.PacketWriter, error) {
			return dm.CreateIsolatedWriterHandle(iname)
		},
	}, nil
}

func (s *TransmitterSink) Name() string {
	return fmt.Sprintf("TransmitterSink(%s)", s.Iname)
}

func (s *TransmitterSink) Write(pkt *proxy.Packet) error {
	if pkt == nil {
		return nil
	}

	s.mu.Lock()
	if s.Writer == nil {
		// Reconnect is in progress; drop this packet.
		s.mu.Unlock()
		return nil
	}

	// Keep the lock while writing so no other goroutine can close/replace the
	// underlying pcap handle concurrently.
	if err := s.Writer.WritePacketData(pkt.Raw); err != nil {
		staleWriter := s.Writer
		slog.Warn("transmitter write failed, initiating reconnect",
			slog.String("interface", s.Iname),
			slog.String("error", err.Error()))

		if !s.reconnecting {
			s.reconnecting = true
			s.Writer = nil
			closePacketWriter(staleWriter)
			go s.reconnectLoop()
		}
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	return nil
}

// reconnectLoop runs in a goroutine and retries opening the writer handle until
// it succeeds or the sink is closed.
func (s *TransmitterSink) reconnectLoop() {
	slog.Warn("waiting for interface to come back",
		slog.String("interface", s.Iname))

	delay := reconnectInitialDelay
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(delay):
		}

		if s.createWriter == nil {
			return
		}

		handle, err := s.createWriter(s.Iname)
		if err != nil {
			slog.Debug("interface not yet available for writing, retrying",
				slog.String("interface", s.Iname),
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

		s.mu.Lock()
		if s.ctx.Err() != nil {
			s.mu.Unlock()
			return
		}
		s.Writer = handle
		s.reconnecting = false
		s.mu.Unlock()
		slog.Info("interface reconnected for writing",
			slog.String("interface", s.Iname))
		return
	}
}

// Close cancels any in-progress reconnect and closes the underlying PCAP handle.
func (s *TransmitterSink) Close() error {
	s.cancel()

	s.mu.Lock()
	staleWriter := s.Writer
	s.Writer = nil
	s.reconnecting = false
	s.mu.Unlock()
	closePacketWriter(staleWriter)
	return nil
}

func closePacketWriter(w proxy.PacketWriter) {
	if w == nil {
		return
	}
	if c, ok := w.(interface{ Close() }); ok {
		c.Close()
	}
}
