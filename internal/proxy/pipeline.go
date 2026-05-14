package proxy

import (
	"context"
	"io"

	log "github.com/sirupsen/logrus"
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
		Source:     source,
		Processors: make([]Processor, 0),
		Sinks:      make([]Sink, 0),
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			pkt, err := p.Source.Read()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				log.Errorf("Source read error: %v", err)
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
					log.Errorf("Processor error: %v", err)
					continue
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
					log.Errorf("Sink write error: %v", err)
				}
			}
		}
	}
}

func (p *Pipeline) closeAll() {
	if err := p.Source.Close(); err != nil {
		log.Errorf("Error closing source: %v", err)
	}
	for _, sink := range p.Sinks {
		if err := sink.Close(); err != nil {
			log.Errorf("Error closing sink: %v", err)
		}
	}
}
