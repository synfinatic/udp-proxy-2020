package proxy

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockSource struct {
	packets []*Packet
	index   int
}

func (m *mockSource) Read() (*Packet, error) {
	if m.index >= len(m.packets) {
		return nil, nil // Return nil to signal end for this test or use EOF
	}
	p := m.packets[m.index]
	m.index++
	return p, nil
}

func (m *mockSource) Close() error { return nil }

type mockProcessor struct {
	keep bool
}

func (m *mockProcessor) Process(pkt *Packet) (bool, error) {
	return m.keep, nil
}

type mockSink struct {
	mu      sync.Mutex
	written []*Packet
}

func (m *mockSink) Write(pkt *Packet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = append(m.written, pkt)
	return nil
}

func (m *mockSink) Close() error { return nil }

func TestPipeline_Run(t *testing.T) {
	pkt := &Packet{Raw: []byte("test")}
	source := &mockSource{packets: []*Packet{pkt}}
	sink := &mockSink{}

	pipeline := NewPipeline(source)
	pipeline.AddSink(sink)

	ctx, cancel := context.WithCancel(context.Background())
	// Ensure we close the source to break the loop even if ctx isn't cancelled
	// but here we want to test that Run consumes the packet and then we stop.

	go func() {
		// Give it a moment to process
		for {
			sink.mu.Lock()
			count := len(sink.written)
			sink.mu.Unlock()
			if count > 0 {
				cancel()
				break
			}
		}
	}()

	_ = pipeline.Run(ctx)

	if len(sink.written) != 1 {
		t.Errorf("Expected 1 packet in sink, got %d", len(sink.written))
	}
}

func TestPipeline_ProcessorFiltering(t *testing.T) {
	pkt := &Packet{Raw: []byte("test")}
	source := &mockSource{packets: []*Packet{pkt, nil}} // nil signals end in mockSource
	sink := &mockSink{}

	pipeline := NewPipeline(source)
	pipeline.AddProcessor(&mockProcessor{keep: false})
	pipeline.AddSink(sink)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(100*time.Millisecond))
	defer cancel()

	_ = pipeline.Run(ctx)

	if len(sink.written) != 0 {
		t.Errorf("Expected 0 packets in sink, got %d", len(sink.written))
	}
}
