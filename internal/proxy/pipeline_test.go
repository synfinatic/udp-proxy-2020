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

func (m *mockSource) Read(ctx context.Context) (*Packet, error) {
	if m.index >= len(m.packets) {
		return nil, context.Canceled // Return canceled to signal end for this test
	}
	p := m.packets[m.index]
	m.index++
	return p, nil
}

func (m *mockSource) Close() error { return nil }

func (m *mockSource) Name() string { return "mockSource" }

type mockProcessor struct {
	keep bool
}

func (m *mockProcessor) Process(pkt *Packet) (bool, error) {
	return m.keep, nil
}

func (m *mockProcessor) Name() string { return "mockProcessor" }

type mockSink struct {
	mu      sync.Mutex
	written []*Packet
	notify  chan struct{}
}

func (m *mockSink) Write(pkt *Packet) error {
	m.mu.Lock()
	m.written = append(m.written, pkt)
	m.mu.Unlock()
	select {
	case m.notify <- struct{}{}:
	default:
	}
	return nil
}

func (m *mockSink) Close() error { return nil }

func (m *mockSink) Name() string { return "mockSink" }

func TestPipeline_Run(t *testing.T) {
	pkt := &Packet{Raw: []byte("test")}
	source := &mockSource{packets: []*Packet{pkt}}
	sink := &mockSink{notify: make(chan struct{}, 1)}

	pipeline := NewPipeline(source)
	pipeline.AddSink(sink)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		select {
		case <-sink.notify:
			cancel()
		case <-ctx.Done():
		}
	}()

	_ = pipeline.Run(ctx)

	if len(sink.written) != 1 {
		t.Errorf("Expected 1 packet in sink, got %d", len(sink.written))
	}
}

func TestPipeline_ProcessorFiltering(t *testing.T) {
	pkt := &Packet{Raw: []byte("test")}
	source := &mockSource{packets: []*Packet{pkt}}
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
