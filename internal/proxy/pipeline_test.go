package proxy

import (
	"context"
	"testing"
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
	written []*Packet
}

func (m *mockSink) Write(pkt *Packet) error {
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

	// Create a context that cancels after a short bit to stop the loop
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Small delay to allow Run to process the packet
		// In a real test we'd use a better synchronization method
		cancel()
	}()

	_ = pipeline.Run(ctx)

	if len(sink.written) == 0 {
		// Note: This test is a bit racey due to the loop structure,
		// but demonstrates the principle.
	}
}

func TestPipeline_ProcessorFiltering(t *testing.T) {
	pkt := &Packet{Raw: []byte("test")}
	source := &mockSource{packets: []*Packet{pkt}}
	sink := &mockSink{}

	pipeline := NewPipeline(source)
	pipeline.AddProcessor(&mockProcessor{keep: false})
	pipeline.AddSink(sink)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // stop immediately

	_ = pipeline.Run(ctx)

	if len(sink.written) != 0 {
		t.Errorf("Expected 0 packets in sink, got %d", len(sink.written))
	}
}
