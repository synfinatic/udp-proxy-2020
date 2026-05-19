package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy/stages"
)

// GenerateDotFile creates a Graphviz dot file representing the pipelines.
func GenerateDotFile(pipelines []*proxy.Pipeline, filepath string) error {
	var buf strings.Builder

	fmt.Fprintf(&buf, "digraph Pipelines {\n")
	fmt.Fprintf(&buf, "  rankdir=LR;\n")
	fmt.Fprintf(&buf, "  node [shape=box, style=rounded];\n\n")

	// Process each pipeline
	for i, pipeline := range pipelines {
		pipelineID := fmt.Sprintf("pipeline_%d", i)

		// Get source name
		sourceName := pipeline.Source.Name()

		// Create node for source
		sourceNodeID := fmt.Sprintf("%s_source", pipelineID)
		fmt.Fprintf(&buf, "  %s [label=\"%s\", style=\"rounded,filled\", fillcolor=\"lightblue\"];\n",
			sourceNodeID, sourceName)

		// Track the previous node for edge creation
		prevNodeID := sourceNodeID

		// Process each processor
		for j, processor := range pipeline.Processors {
			processorNodeID := fmt.Sprintf("%s_proc_%d", pipelineID, j)
			processorName := processor.Name()
			fmt.Fprintf(&buf, "  %s [label=\"%s\", style=\"rounded,filled\", fillcolor=\"lightyellow\"];\n",
				processorNodeID, processorName)
			fmt.Fprintf(&buf, "  %s -> %s;\n", prevNodeID, processorNodeID)
			prevNodeID = processorNodeID
		}

		// Process each sink
		for j, sink := range pipeline.Sinks {
			sinkNodeID := fmt.Sprintf("%s_sink_%d", pipelineID, j)
			sinkName := sink.Name()

			// Determine fill color based on sink type
			fillColor := "lightgreen"
			if isForwardingSink(sink) {
				fillColor = "lightcoral"
			}

			fmt.Fprintf(&buf, "  %s [label=\"%s\", style=\"rounded,filled\", fillcolor=\"%s\"];\n",
				sinkNodeID, sinkName, fillColor)
			fmt.Fprintf(&buf, "  %s -> %s;\n", prevNodeID, sinkNodeID)
		}

		fmt.Fprintf(&buf, "\n")
	}

	// Add cross-pipeline forwarding edges
	addCrossPipelineEdges(&buf, pipelines)

	fmt.Fprintf(&buf, "}\n")

	// Write to file with secure permissions (0600)
	if err := os.WriteFile(filepath, []byte(buf.String()), 0600); err != nil {
		return fmt.Errorf("failed to write dot file: %w", err)
	}

	return nil
}

// isForwardingSink checks if a sink is a ForwardingSink by type checking
func isForwardingSink(sink proxy.Sink) bool {
	_, ok := sink.(*stages.ForwardingSink)
	return ok
}

// addCrossPipelineEdges creates edges for cross-pipeline forwarding
func addCrossPipelineEdges(buf *strings.Builder, pipelines []*proxy.Pipeline) {
	// Map interface names to pipeline indices
	ifaceToIndex := make(map[string]int)
	for i, pipeline := range pipelines {
		sourceName := pipeline.Source.Name()
		// Extract interface name from source (e.g., "PcapSource(eth0)" -> "eth0")
		if strings.Contains(sourceName, "(") && strings.Contains(sourceName, ")") {
			start := strings.Index(sourceName, "(") + 1
			end := strings.Index(sourceName, ")")
			if start > 0 && end > start {
				iface := sourceName[start:end]
				ifaceToIndex[iface] = i
			}
		}
	}

	// Look for ForwardingSinks and create edges
	for i, pipeline := range pipelines {
		for j, sink := range pipeline.Sinks {
			if fSink, ok := sink.(*stages.ForwardingSink); ok {
				// Extract destination interface from ForwardingSink
				sinkName := fSink.Name()
				if strings.Contains(sinkName, "(") && strings.Contains(sinkName, ")") {
					start := strings.Index(sinkName, "(") + 1
					end := strings.Index(sinkName, ")")
					if start > 0 && end > start {
						destIface := sinkName[start:end]
						if destIdx, ok := ifaceToIndex[destIface]; ok {
							sinkNodeID := fmt.Sprintf("pipeline_%d_sink_%d", i, j)
							sourceNodeID := fmt.Sprintf("pipeline_%d_source", destIdx)
							fmt.Fprintf(buf, "  %s -> %s [style=dashed, color=red, label=\"forward\"];\n", sinkNodeID, sourceNodeID)
						}
					}
				}
			}
		}
	}
}
