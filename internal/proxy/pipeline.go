package proxy

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// Pipeline orchestrates the flow of packets from a Source through Processors to Sinks.
type Pipeline struct {
	Source     Source
	Processors []Processor
	Sinks      []Sink
}

// NewPipeline creates a new pipeline with the given source.
func NewPipeline(source Source) *Pipeline {
	return &Pipeline{
		Source: source,
	}
}

// AddProcessor adds a processor to the pipeline.
func (p *Pipeline) AddProcessor(proc Processor) {
	p.Processors = append(p.Processors, proc)
}

// AddSink adds a sink to the pipeline.
func (p *Pipeline) AddSink(sink Sink) {
	p.Sinks = append(p.Sinks, sink)
}

// Run starts the pipeline and processes packets until the context is cancelled or the source is closed.
func (p *Pipeline) Run(ctx context.Context) error {
	defer p.closeAll()

	for {
		pkt, err := p.Source.Read(ctx)
		if err != nil {
			if err == io.EOF || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			slog.Error("Source read error", "error", err)
			continue
		}

		if pkt == nil {
			continue
		}

		// Process packet through processors
		continueProcessing := true
		for _, proc := range p.Processors {
			keep, err := proc.Process(pkt)
			if err != nil {
				slog.Error("Processor error", "error", err)
				continueProcessing = false
				break
			}
			if !keep {
				continueProcessing = false
				break
			}
		}

		if !continueProcessing {
			continue
		}

		// Write to sinks
		for _, sink := range p.Sinks {
			if err := sink.Write(pkt); err != nil {
				slog.Error("Sink write error", "error", err)
			}
		}
	}
}

func (p *Pipeline) closeAll() {
	if err := p.Source.Close(); err != nil {
		slog.Error("Error closing source", "error", err)
	}
	for _, sink := range p.Sinks {
		if err := sink.Close(); err != nil {
			slog.Error("Error closing sink", "error", err)
		}
	}
}
