package stages

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
)

type mockWriter struct {
	data     []byte
	linkType layers.LinkType
	writeErr error
}

func (m *mockWriter) WritePacketData(data []byte) error {
	m.data = data
	return m.writeErr
}

func (m *mockWriter) LinkType() layers.LinkType {
	return m.linkType
}

func newTestTransmitter(w proxy.PacketWriter) *TransmitterSink {
	ctx, cancel := context.WithCancel(context.Background())
	return &TransmitterSink{
		Writer: w,
		Iname:  "eth0",
		ctx:    ctx,
		cancel: cancel,
	}
}

func TestTransmitterSink_Write(t *testing.T) {
	writer := &mockWriter{linkType: layers.LinkTypeEthernet}
	s := newTestTransmitter(writer)

	pkt := &proxy.Packet{Raw: []byte("payload")}
	if err := s.Write(pkt); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if writer.data == nil {
		t.Fatal("Expected data to be written to writer, but got nil")
	}
}

func TestTransmitterSink_WriteNilPacket(t *testing.T) {
	writer := &mockWriter{linkType: layers.LinkTypeEthernet}
	s := newTestTransmitter(writer)

	if err := s.Write(nil); err != nil {
		t.Fatalf("Write(nil) failed: %v", err)
	}

	if writer.data == nil {
		return
	}
}

// TestTransmitterSink_WriteErrorTriggersReconnect verifies that a write failure
// sets Writer to nil (dropping subsequent packets) and marks reconnecting.
func TestTransmitterSink_WriteErrorTriggersReconnect(t *testing.T) {
	writer := &mockWriter{
		linkType: layers.LinkTypeEthernet,
		writeErr: errors.New("interface gone"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the goroutine doesn't spin or touch pcap
	dm := &proxy.DeviceManager{}
	s := &TransmitterSink{
		dm:     dm,
		Writer: writer,
		Iname:  "eth0",
		ctx:    ctx,
		cancel: cancel,
	}

	pkt := &proxy.Packet{Raw: []byte("payload")}
	if err := s.Write(pkt); err != nil {
		t.Fatalf("Write should not return an error: %v", err)
	}

	s.mu.Lock()
	writerNil := s.Writer == nil
	reconnecting := s.reconnecting
	s.mu.Unlock()

	if !writerNil {
		t.Error("expected Writer to be nil after write error")
	}
	if !reconnecting {
		t.Error("expected reconnecting to be true after write error")
	}
}

// TestTransmitterSink_WriteWhileReconnecting verifies that packets are silently
// dropped (no error, no panic) while Writer is nil.
func TestTransmitterSink_WriteWhileReconnecting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &TransmitterSink{
		Writer:       nil, // simulates mid-reconnect state
		Iname:        "eth0",
		reconnecting: true,
		ctx:          ctx,
		cancel:       cancel,
	}

	pkt := &proxy.Packet{Raw: []byte("payload")}
	if err := s.Write(pkt); err != nil {
		t.Fatalf("Write while reconnecting should not return an error: %v", err)
	}
}

// TestTransmitterSink_ReconnectLoopExitsOnCancel verifies that reconnectLoop
// exits promptly when the sink's context is cancelled, without panicking even
// when dm is nil (only safe because the loop exits before touching dm).
func TestTransmitterSink_ReconnectLoopExitsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &TransmitterSink{
		dm:           nil, // not reached because ctx is cancelled before first retry
		Writer:       nil,
		Iname:        "eth0",
		reconnecting: true,
		ctx:          ctx,
		cancel:       cancel,
	}

	cancel() // cancel before the goroutine even starts its first wait
	done := make(chan struct{})
	go func() {
		s.reconnectLoop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("reconnectLoop did not exit after context cancellation")
	}
}

func TestTransmitterSink_WriteErrorWithNilDeviceManagerDoesNotPanic(t *testing.T) {
	writer := &mockWriter{
		linkType: layers.LinkTypeEthernet,
		writeErr: errors.New("device not configured"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ensure reconnectLoop exits immediately

	s := &TransmitterSink{
		dm:     nil,
		Writer: writer,
		Iname:  "eth0",
		ctx:    ctx,
		cancel: cancel,
	}

	pkt := &proxy.Packet{Raw: []byte("payload")}
	if err := s.Write(pkt); err != nil {
		t.Fatalf("Write should not return error: %v", err)
	}
}
